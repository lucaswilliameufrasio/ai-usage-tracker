// Package service installs the agent as a per-user background service:
// a systemd user unit on Linux and a LaunchAgent on macOS.
package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"ai-usage-tracker/internal/agent"
)

const (
	UnitName = "ai-usage.service"
	Label    = "dev.ai-usage.agent"
)

var (
	ErrUnsupported = errors.New("service management supports Linux (systemd) and macOS (launchd) only")
	ErrNotReady    = errors.New("the agent is not configured; run `ai-usage setup` first")
)

// Runner runs a command and returns its combined output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Manager holds everything that differs between machines, so tests can fake it.
type Manager struct {
	GOOS     string
	Home     string
	UID      int
	User     string
	Exe      string
	Runner   Runner
	LookPath func(file string) (string, error)
}

type Report struct {
	File     string
	Content  string
	Commands []string
	Notes    []string
	Changed  bool
}

type InstallOptions struct {
	DryRun bool
}

func (m Manager) kind() (string, error) {
	switch m.GOOS {
	case "linux":
		return "systemd", nil
	case "darwin":
		return "launchd", nil
	}
	return "", ErrUnsupported
}

func (m Manager) FilePath() (string, error) {
	kind, err := m.kind()
	if err != nil {
		return "", err
	}
	if kind == "systemd" {
		return filepath.Join(m.Home, ".config", "systemd", "user", UnitName), nil
	}
	return filepath.Join(m.Home, "Library", "LaunchAgents", Label+".plist"), nil
}

func (m Manager) LogPath() string {
	return filepath.Join(m.Home, "Library", "Logs", "ai-usage.log")
}

func (m Manager) domain() string {
	return fmt.Sprintf("gui/%d", m.UID)
}

// ServicePATH builds the PATH the service runs with. Services start with a minimal PATH,
// so the folders of the tools found in the installing shell are added.
func (m Manager) ServicePATH(cfg agent.File) string {
	var dirs []string
	seen := map[string]bool{}
	add := func(dir string) {
		if dir == "" || !filepath.IsAbs(dir) || seen[dir] {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	add(filepath.Dir(m.Exe))
	tool := func(name string) {
		if name == "" || m.LookPath == nil {
			return
		}
		found, err := m.LookPath(name)
		if err != nil {
			return
		}
		add(filepath.Dir(found))
		if node, err := m.LookPath("node"); err == nil {
			add(filepath.Dir(node))
		}
	}
	if cfg.CodexDir != "" {
		tool("codex")
	}
	tool(cfg.AgyCommand)
	add(filepath.Join(m.Home, ".local", "bin"))
	if m.GOOS == "darwin" {
		add("/opt/homebrew/bin")
	}
	for _, dir := range []string{"/usr/local/bin", "/usr/bin", "/bin"} {
		add(dir)
	}
	return strings.Join(dirs, ":")
}

func (m Manager) Render(cfg agent.File) (string, error) {
	kind, err := m.kind()
	if err != nil {
		return "", err
	}
	for _, value := range []string{m.Exe, m.Home} {
		if strings.ContainsAny(value, "\n\r") {
			return "", errors.New("path contains a line break")
		}
	}
	path := m.ServicePATH(cfg)
	if kind == "systemd" {
		return renderUnit(m.Exe, path), nil
	}
	return renderPlist(m.Exe, path, m.LogPath()), nil
}

func renderUnit(exe string, path string) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=AI usage agent",
		"",
		"[Service]",
		"ExecStart=" + unitQuote(exe, true) + " run",
		"Environment=" + unitQuote("PATH="+path, false),
		"Restart=on-failure",
		"RestartSec=60",
		"RestartPreventExitStatus=3",
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	}, "\n")
}

func unitQuote(value string, exec bool) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "%", "%%")
	if exec {
		value = strings.ReplaceAll(value, "$", "$$")
	}
	return `"` + value + `"`
}

func xmlText(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}

func renderPlist(exe string, path string, logPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + Label + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + xmlText(exe) + `</string>
    <string>run</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>` + xmlText(path) + `</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key>
    <false/>
  </dict>
  <key>ThrottleInterval</key>
  <integer>60</integer>
  <key>StandardOutPath</key>
  <string>` + xmlText(logPath) + `</string>
  <key>StandardErrorPath</key>
  <string>` + xmlText(logPath) + `</string>
</dict>
</plist>
`
}

var (
	plistExe  = regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>\s*<string>(.*?)</string>`)
	plistPath = regexp.MustCompile(`(?s)<key>PATH</key>\s*<string>(.*?)</string>`)
)

// InstalledExe returns the binary the installed service file points at.
func InstalledExe(content string) string {
	if match := plistExe.FindStringSubmatch(content); match != nil {
		return html.UnescapeString(match[1])
	}
	for _, line := range strings.Split(content, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "ExecStart="); ok {
			return firstToken(rest)
		}
	}
	return ""
}

// InstalledPATH returns the PATH the installed service file sets.
func InstalledPATH(content string) string {
	if match := plistPath.FindStringSubmatch(content); match != nil {
		return html.UnescapeString(match[1])
	}
	for _, line := range strings.Split(content, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Environment="); ok {
			return strings.TrimPrefix(firstToken(rest), "PATH=")
		}
	}
	return ""
}

func firstToken(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, `"`) {
		token, _, _ := strings.Cut(text, " ")
		return token
	}
	var out strings.Builder
	for i := 1; i < len(text); i++ {
		c := text[i]
		if c == '\\' && i+1 < len(text) {
			out.WriteByte(text[i+1])
			i++
			continue
		}
		if c == '"' {
			break
		}
		out.WriteByte(c)
	}
	return strings.ReplaceAll(strings.ReplaceAll(out.String(), "%%", "%"), "$$", "$")
}

func (m Manager) check(cfg agent.File) error {
	if !cfg.Ready() {
		return ErrNotReady
	}
	info, err := os.Stat(cfg.TokenFile)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return errors.New("the device token is missing; run `ai-usage setup` to enroll this device")
	}
	if !filepath.IsAbs(m.Exe) {
		return errors.New("the binary path is not absolute")
	}
	if strings.Contains(m.Exe, "go-build") {
		return errors.New("this is a temporary `go run` binary; install ai-usage first and run it from there")
	}
	return nil
}

// Install writes the service file and starts the service.
func (m Manager) Install(ctx context.Context, cfg agent.File, opts InstallOptions) (Report, error) {
	kind, err := m.kind()
	if err != nil {
		return Report{}, err
	}
	if err := m.check(cfg); err != nil {
		return Report{}, err
	}
	content, err := m.Render(cfg)
	if err != nil {
		return Report{}, err
	}
	file, _ := m.FilePath()
	report := Report{File: file, Content: content}
	_, statErr := os.Stat(file)
	existed := statErr == nil
	var steps [][]string
	if kind == "systemd" {
		steps = [][]string{
			{"systemctl", "--user", "daemon-reload"},
			{"systemctl", "--user", "enable", UnitName},
			{"systemctl", "--user", "restart", UnitName},
		}
	} else {
		if existed {
			steps = append(steps, []string{"launchctl", "bootout", m.domain() + "/" + Label})
		}
		steps = append(steps,
			[]string{"launchctl", "bootstrap", m.domain(), file},
			[]string{"launchctl", "kickstart", "-k", m.domain() + "/" + Label},
		)
	}
	for _, step := range steps {
		report.Commands = append(report.Commands, strings.Join(step, " "))
	}
	if opts.DryRun {
		return report, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return report, fmt.Errorf("could not create %s: %w", filepath.Dir(file), err)
	}
	if kind == "launchd" {
		if err := os.MkdirAll(filepath.Dir(m.LogPath()), 0o755); err != nil {
			return report, fmt.Errorf("could not create the log folder: %w", err)
		}
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return report, fmt.Errorf("could not write %s: %w", file, err)
	}
	report.Changed = true
	for i, step := range steps {
		out, err := m.Runner.Run(ctx, step[0], step[1:]...)
		if err != nil {
			if step[1] == "bootout" {
				continue
			}
			return report, commandError(kind, report.Commands[i], out, err, i == 0)
		}
	}
	report.Notes = m.afterInstall(ctx, kind)
	return report, nil
}

func commandError(kind string, line string, out string, err error, first bool) error {
	message := fmt.Sprintf("`%s` failed: %v", line, err)
	if out != "" {
		message += ": " + out
	}
	if kind == "systemd" && first {
		message += " (the systemd user session is not reachable; log in with a full session, or set XDG_RUNTIME_DIR=/run/user/$(id -u))"
	}
	return errors.New(message)
}

func (m Manager) afterInstall(ctx context.Context, kind string) []string {
	if kind != "systemd" {
		return []string{"Logs: tail -f " + m.LogPath()}
	}
	notes := []string{"Logs: journalctl --user -u ai-usage -f"}
	if m.lingerOff(ctx) {
		notes = append(notes, "Linger is off, so the agent stops when you log out. To keep it running: loginctl enable-linger "+m.userName())
	}
	return notes
}

func (m Manager) userName() string {
	if m.User != "" {
		return m.User
	}
	return "$USER"
}

func (m Manager) lingerOff(ctx context.Context) bool {
	out, err := m.Runner.Run(ctx, "loginctl", "show-user", fmt.Sprint(m.UID), "-p", "Linger", "--value")
	return err == nil && strings.TrimSpace(out) == "no"
}

// Uninstall stops the service and removes only its file. Configuration, token and queue stay.
func (m Manager) Uninstall(ctx context.Context) (Report, error) {
	kind, err := m.kind()
	if err != nil {
		return Report{}, err
	}
	file, _ := m.FilePath()
	report := Report{File: file}
	_, statErr := os.Stat(file)
	present := statErr == nil
	if kind == "systemd" {
		if present {
			_, _ = m.Runner.Run(ctx, "systemctl", "--user", "disable", "--now", UnitName)
		}
	} else {
		_, _ = m.Runner.Run(ctx, "launchctl", "bootout", m.domain()+"/"+Label)
	}
	if present {
		if err := os.Remove(file); err != nil {
			return report, fmt.Errorf("could not remove %s: %w", file, err)
		}
		report.Changed = true
	}
	if kind == "systemd" && present {
		_, _ = m.Runner.Run(ctx, "systemctl", "--user", "daemon-reload")
		_, _ = m.Runner.Run(ctx, "systemctl", "--user", "reset-failed", UnitName)
	}
	if report.Changed {
		report.Notes = []string{"Your configuration, device token and local queue were kept."}
	}
	return report, nil
}
