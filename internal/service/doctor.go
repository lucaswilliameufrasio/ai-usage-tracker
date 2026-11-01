package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ai-usage-tracker/internal/agent"
	"ai-usage-tracker/internal/spool"
)

type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
)

type Check struct {
	Name   string
	Status Status
	Detail string
	Fix    string
}

// DoctorInput is what the command already loaded. The doctor only reads.
type DoctorInput struct {
	Config     agent.File
	ConfigPath string
	ConfigErr  error
	HTTP       *http.Client
}

var launchState = regexp.MustCompile(`state = (\w+)`)

func Failed(checks []Check) bool {
	for _, check := range checks {
		if check.Status == Fail {
			return true
		}
	}
	return false
}

func (m Manager) Doctor(ctx context.Context, in DoctorInput) []Check {
	var checks []Check
	add := func(name string, status Status, detail string, fix string) {
		checks = append(checks, Check{Name: name, Status: status, Detail: detail, Fix: fix})
	}
	kind, err := m.kind()
	if err != nil {
		add("platform", Fail, fmt.Sprintf("%s is not supported", m.GOOS), "use Linux or macOS (WSL2 works as Linux)")
		return checks
	}
	tool := "systemctl"
	label := "systemd user service"
	if kind == "launchd" {
		tool = "launchctl"
		label = "launchd agent"
	}
	if m.LookPath != nil {
		if _, err := m.LookPath(tool); err != nil {
			add("platform", Fail, tool+" not found", "install "+tool+" or use a distribution with systemd")
			return checks
		}
	}
	add("platform", OK, label, "")

	cfg := in.Config
	ready := false
	switch {
	case in.ConfigErr != nil && os.IsNotExist(in.ConfigErr):
		add("config", Fail, "no configuration at "+in.ConfigPath, "run: ai-usage setup")
	case in.ConfigErr != nil:
		add("config", Fail, "configuration unreadable at "+in.ConfigPath, "run: ai-usage setup")
	case !cfg.Ready():
		add("config", Fail, "configuration is incomplete", "run: ai-usage setup")
	default:
		ready = true
		add("config", OK, in.ConfigPath, "")
	}
	if ready {
		checks = append(checks, m.tokenCheck(cfg))
	}

	file, _ := m.FilePath()
	raw, readErr := os.ReadFile(file)
	installed := readErr == nil
	if !installed {
		add("service file", Fail, "not installed ("+file+")", "run: ai-usage service install")
	} else {
		add("service file", OK, file, "")
		pointed := InstalledExe(string(raw))
		switch {
		case pointed == "":
			add("binary", Fail, "the service file has no command", "run: ai-usage service install")
		case pointed != m.Exe:
			add("binary", Warn, "the service runs "+pointed+", this is "+m.Exe, "run: ai-usage service install")
		default:
			if _, err := os.Stat(pointed); err != nil {
				add("binary", Fail, pointed+" does not exist", "run: ai-usage service install")
			} else {
				add("binary", OK, pointed, "")
			}
		}
		checks = append(checks, m.stateChecks(ctx, kind)...)
	}
	if kind == "systemd" && m.lingerOff(ctx) {
		add("linger", Warn, "the agent stops when you log out", "run: loginctl enable-linger "+m.userName())
	} else if kind == "systemd" {
		add("linger", OK, "the agent keeps running after logout", "")
	}
	if installed && ready {
		checks = append(checks, m.toolChecks(cfg, InstalledPATH(string(raw)))...)
	}
	if ready {
		server := m.serverCheck(ctx, in, cfg)
		checks = append(checks, server)
		if server.Status == OK {
			checks = append(checks, m.deviceCheck(ctx, in, cfg))
		}
		checks = append(checks, m.queueCheck(cfg))
	}
	return checks
}

func (m Manager) tokenCheck(cfg agent.File) Check {
	info, err := os.Stat(cfg.TokenFile)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return Check{Name: "token", Status: Fail, Detail: "missing at " + cfg.TokenFile, Fix: "run: ai-usage setup"}
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Check{Name: "token", Status: Warn, Detail: fmt.Sprintf("%s is readable by others (%v)", cfg.TokenFile, info.Mode().Perm()), Fix: "run: chmod 600 " + cfg.TokenFile}
	}
	return Check{Name: "token", Status: OK, Detail: cfg.TokenFile}
}

func (m Manager) stateChecks(ctx context.Context, kind string) []Check {
	if kind == "systemd" {
		active, _ := m.Runner.Run(ctx, "systemctl", "--user", "is-active", UnitName)
		enabled, _ := m.Runner.Run(ctx, "systemctl", "--user", "is-enabled", UnitName)
		var checks []Check
		if strings.TrimSpace(active) == "active" {
			checks = append(checks, Check{Name: "service", Status: OK, Detail: "active"})
		} else {
			state := strings.TrimSpace(active)
			if state == "" {
				state = "unknown"
			}
			checks = append(checks, Check{Name: "service", Status: Fail, Detail: state, Fix: "run: journalctl --user -u ai-usage -n 30"})
		}
		if strings.TrimSpace(enabled) != "enabled" {
			checks = append(checks, Check{Name: "autostart", Status: Warn, Detail: "not enabled at login", Fix: "run: ai-usage service install"})
		} else {
			checks = append(checks, Check{Name: "autostart", Status: OK, Detail: "enabled at login"})
		}
		return checks
	}
	out, err := m.Runner.Run(ctx, "launchctl", "print", m.domain()+"/"+Label)
	if err != nil {
		return []Check{{Name: "service", Status: Fail, Detail: "not loaded", Fix: "run: ai-usage service install"}}
	}
	state := "unknown"
	if match := launchState.FindStringSubmatch(out); match != nil {
		state = match[1]
	}
	if state == "running" {
		return []Check{{Name: "service", Status: OK, Detail: "running"}}
	}
	return []Check{{Name: "service", Status: Fail, Detail: "loaded but " + state, Fix: "run: tail -n 30 " + m.LogPath()}}
}

// toolChecks looks up codex, agy and node on the PATH the service was installed with.
func (m Manager) toolChecks(cfg agent.File, servicePath string) []Check {
	var checks []Check
	look := func(name string) bool {
		if filepath.IsAbs(name) {
			_, err := exec.LookPath(name)
			return err == nil
		}
		for _, dir := range filepath.SplitList(servicePath) {
			if _, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
				return true
			}
		}
		return false
	}
	need := func(label string, name string, why string) {
		if look(name) {
			checks = append(checks, Check{Name: label, Status: OK, Detail: name + " is on the service PATH"})
			return
		}
		checks = append(checks, Check{
			Name: label, Status: Warn, Detail: name + " is not on the service PATH; " + why,
			Fix: "open a shell where `" + name + "` works and run: ai-usage service install",
		})
	}
	if cfg.CodexDir != "" {
		need("codex", "codex", "the live Codex read falls back to the file")
	}
	if cfg.AgyCommand != "" {
		need("antigravity", cfg.AgyCommand, "no Antigravity sample will be sent")
	}
	return checks
}

func (m Manager) serverCheck(ctx context.Context, in DoctorInput, cfg agent.File) Check {
	client := in.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	url := strings.TrimRight(cfg.API, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Check{Name: "server", Status: Fail, Detail: "invalid address " + cfg.API, Fix: "run: ai-usage setup"}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Check{Name: "server", Status: Fail, Detail: "cannot reach " + cfg.API, Fix: "check the tracker is running and the address is right"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Check{Name: "server", Status: Fail, Detail: fmt.Sprintf("%s answered %d", url, resp.StatusCode), Fix: "check the tracker logs"}
	}
	return Check{Name: "server", Status: OK, Detail: cfg.API}
}

// deviceCheck asks the server whether it accepts the saved token. A token from another
// server, or a revoked one, passes every local check and still fails here.
func (m Manager) deviceCheck(ctx context.Context, in DoctorInput, cfg agent.File) Check {
	raw, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return Check{Name: "device", Status: Fail, Detail: "token unreadable", Fix: "run: ai-usage setup"}
	}
	result, err := agent.CheckToken(ctx, in.HTTP, cfg.API, strings.TrimSpace(string(raw)))
	if err != nil {
		return Check{Name: "device", Status: Warn, Detail: "could not ask the server about the token", Fix: "run: ai-usage service doctor again"}
	}
	const enroll = "create a code under Devices on this server and run: ai-usage setup"
	switch result.State {
	case agent.TokenAccepted:
		detail := "the server accepts this device token"
		if result.Device != "" {
			detail = "the server knows this device as " + result.Device
		}
		return Check{Name: "device", Status: OK, Detail: detail}
	case agent.TokenRevoked:
		return Check{Name: "device", Status: Fail, Detail: "this device was revoked on the server", Fix: enroll}
	case agent.TokenRefused:
		return Check{Name: "device", Status: Fail, Detail: "the server does not accept this token; it was probably issued by another server", Fix: enroll}
	}
	return Check{Name: "device", Status: Warn, Detail: "this server is too old to verify the token", Fix: "update the tracker to the version that has GET /v1/device"}
}

func (m Manager) queueCheck(cfg agent.File) Check {
	items, err := spool.Load(cfg.Spool)
	if err != nil {
		return Check{Name: "queue", Status: Warn, Detail: "local queue unreadable at " + cfg.Spool, Fix: "run: ai-usage run"}
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[item.State]++
	}
	if counts[spool.StateBlocked] > 0 {
		return Check{Name: "queue", Status: Fail, Detail: fmt.Sprintf("%d items blocked: this device was revoked", counts[spool.StateBlocked]), Fix: "create a new code under Devices and run: ai-usage setup"}
	}
	if counts[spool.StateRejected] > 0 {
		return Check{Name: "queue", Status: Warn, Detail: fmt.Sprintf("%d items were rejected by the server and kept", counts[spool.StateRejected]), Fix: "run: ai-usage run and read the per-item lines"}
	}
	if counts[spool.StatePending] > 0 {
		return Check{Name: "queue", Status: Warn, Detail: fmt.Sprintf("%d items waiting to be sent", counts[spool.StatePending]), Fix: "they go out on the next pass"}
	}
	return Check{Name: "queue", Status: OK, Detail: "empty"}
}
