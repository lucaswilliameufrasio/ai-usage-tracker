package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-usage-tracker/internal/agent"
	"ai-usage-tracker/internal/spool"
)

type fakeRunner struct {
	calls   []string
	answers map[string]string
	fail    map[string]bool
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, line)
	if f.fail[line] {
		return "boom", errors.New("exit 1")
	}
	return f.answers[line], nil
}

func lookIn(paths map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if found, ok := paths[name]; ok {
			return found, nil
		}
		return "", errors.New("not found")
	}
}

func setup(t *testing.T, goos string) (Manager, agent.File, *fakeRunner) {
	t.Helper()
	home := t.TempDir()
	token := filepath.Join(home, "device.token")
	if err := os.WriteFile(token, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{answers: map[string]string{}, fail: map[string]bool{}}
	manager := Manager{
		GOOS: goos, Home: home, UID: 1000, User: "ana", Exe: "/opt/ai usage/ai-usage", Runner: runner,
		LookPath: lookIn(map[string]string{"codex": "/home/ana/.nvm/bin/codex", "node": "/home/ana/.nvm/bin/node", "systemctl": "/usr/bin/systemctl", "launchctl": "/bin/launchctl"}),
	}
	cfg := agent.File{API: "http://127.0.0.1:8787", TokenFile: token, Spool: filepath.Join(home, "spool.json"), CodexDir: "/home/ana/.codex/sessions"}
	return manager, cfg, runner
}

func TestRenderUnitQuotesThePathAndKeepsTheToolFolders(t *testing.T) {
	manager, cfg, _ := setup(t, "linux")
	content, err := manager.Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`ExecStart="/opt/ai usage/ai-usage" run`,
		`RestartPreventExitStatus=3`,
		`/home/ana/.nvm/bin`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("falta %q em\n%s", want, content)
		}
	}
	if InstalledExe(content) != manager.Exe {
		t.Fatalf("exe %q", InstalledExe(content))
	}
	if !strings.Contains(InstalledPATH(content), "/home/ana/.nvm/bin") || !strings.Contains(InstalledPATH(content), "/usr/bin") {
		t.Fatalf("path %q", InstalledPATH(content))
	}
}

func TestRenderPlistEscapesAndRoundTrips(t *testing.T) {
	manager, cfg, _ := setup(t, "darwin")
	manager.Exe = "/Users/a&b/bin/ai-usage"
	content, err := manager.Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "a&amp;b") || strings.Contains(content, "a&b") {
		t.Fatalf("sem escape:\n%s", content)
	}
	if InstalledExe(content) != manager.Exe {
		t.Fatalf("exe %q", InstalledExe(content))
	}
	if !strings.Contains(InstalledPATH(content), "/opt/homebrew/bin") {
		t.Fatalf("path %q", InstalledPATH(content))
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	manager, cfg, _ := setup(t, "windows")
	if _, err := manager.Install(context.Background(), cfg, InstallOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("erro %v", err)
	}
	if _, err := manager.Uninstall(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("erro %v", err)
	}
	checks := manager.Doctor(context.Background(), DoctorInput{Config: cfg})
	if len(checks) != 1 || checks[0].Status != Fail {
		t.Fatalf("checks %+v", checks)
	}
}

func TestInstallRefusesWhatCannotWork(t *testing.T) {
	manager, cfg, _ := setup(t, "linux")
	notReady := cfg
	notReady.CodexDir = ""
	if _, err := manager.Install(context.Background(), notReady, InstallOptions{}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("erro %v", err)
	}
	noToken := cfg
	noToken.TokenFile = filepath.Join(t.TempDir(), "none")
	if _, err := manager.Install(context.Background(), noToken, InstallOptions{}); err == nil {
		t.Fatal("instalou sem token")
	}
	temp := manager
	temp.Exe = "/tmp/go-build123/b001/exe/agent"
	if _, err := temp.Install(context.Background(), cfg, InstallOptions{}); err == nil {
		t.Fatal("instalou o binário do go run")
	}
	relative := manager
	relative.Exe = "ai-usage"
	if _, err := relative.Install(context.Background(), cfg, InstallOptions{}); err == nil {
		t.Fatal("instalou caminho relativo")
	}
}

func TestInstallDryRunChangesNothing(t *testing.T) {
	manager, cfg, runner := setup(t, "linux")
	report, err := manager.Install(context.Background(), cfg, InstallOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(report.File); err == nil {
		t.Fatal("gravou o arquivo")
	}
	if len(runner.calls) != 0 || len(report.Commands) != 3 || report.Content == "" {
		t.Fatalf("calls %v report %+v", runner.calls, report)
	}
}

func TestInstallWritesStartsAndWarnsAboutLinger(t *testing.T) {
	manager, cfg, runner := setup(t, "linux")
	runner.answers["loginctl show-user 1000 -p Linger --value"] = "no"
	report, err := manager.Install(context.Background(), cfg, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(report.File)
	if err != nil || !strings.Contains(string(raw), "ExecStart=") {
		t.Fatalf("arquivo %v %s", err, raw)
	}
	want := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable ai-usage.service",
		"systemctl --user restart ai-usage.service",
		"loginctl show-user 1000 -p Linger --value",
	}
	if strings.Join(runner.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("comandos %v", runner.calls)
	}
	if !strings.Contains(strings.Join(report.Notes, "\n"), "loginctl enable-linger ana") {
		t.Fatalf("notas %v", report.Notes)
	}
}

func TestInstallOnMacBootsOutOnlyWhenReinstalling(t *testing.T) {
	manager, cfg, runner := setup(t, "darwin")
	if _, err := manager.Install(context.Background(), cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(runner.calls, "|"), "bootout") {
		t.Fatalf("bootout na primeira vez: %v", runner.calls)
	}
	runner.calls = nil
	runner.fail["launchctl bootout gui/1000/dev.ai-usage.agent"] = true
	if _, err := manager.Install(context.Background(), cfg, InstallOptions{}); err != nil {
		t.Fatalf("bootout com erro derrubou a reinstalação: %v", err)
	}
	if len(runner.calls) != 3 || !strings.HasPrefix(runner.calls[0], "launchctl bootout") {
		t.Fatalf("comandos %v", runner.calls)
	}
}

func TestInstallExplainsAnUnreachableUserSession(t *testing.T) {
	manager, cfg, runner := setup(t, "linux")
	runner.fail["systemctl --user daemon-reload"] = true
	_, err := manager.Install(context.Background(), cfg, InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Fatalf("erro %v", err)
	}
}

func TestUninstallRemovesOnlyTheServiceAndIsRepeatable(t *testing.T) {
	manager, cfg, runner := setup(t, "linux")
	again, err := manager.Uninstall(context.Background())
	if err != nil || again.Changed || len(runner.calls) != 0 {
		t.Fatalf("sem serviço: %v %+v %v", err, again, runner.calls)
	}
	if _, err := manager.Install(context.Background(), cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	runner.calls = nil
	report, err := manager.Uninstall(context.Background())
	if err != nil || !report.Changed {
		t.Fatalf("%v %+v", err, report)
	}
	if _, err := os.Stat(report.File); err == nil {
		t.Fatal("o arquivo do serviço ficou")
	}
	if _, err := os.Stat(cfg.TokenFile); err != nil {
		t.Fatal("apagou o token")
	}
	if len(runner.calls) < 1 || runner.calls[0] != "systemctl --user disable --now ai-usage.service" {
		t.Fatalf("comandos %v", runner.calls)
	}
}

func statuses(checks []Check) map[string]Status {
	out := map[string]Status{}
	for _, check := range checks {
		out[check.Name] = check.Status
	}
	return out
}

func healthy(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(server.Close)
	return server
}

func TestDoctorSeesAHealthyInstall(t *testing.T) {
	manager, cfg, runner := setup(t, "linux")
	server := healthy(t)
	cfg.API = server.URL
	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "codex"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager.Exe = filepath.Join(fakeBin, "ai-usage")
	if err := os.WriteFile(manager.Exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Install(context.Background(), cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	runner.answers["systemctl --user is-active ai-usage.service"] = "active"
	runner.answers["systemctl --user is-enabled ai-usage.service"] = "enabled"
	runner.answers["loginctl show-user 1000 -p Linger --value"] = "yes"
	checks := manager.Doctor(context.Background(), DoctorInput{Config: cfg, ConfigPath: "agent.json"})
	if Failed(checks) {
		t.Fatalf("checks %+v", checks)
	}
	got := statuses(checks)
	for _, name := range []string{"platform", "config", "token", "service file", "binary", "service", "autostart", "linger", "server", "queue"} {
		if got[name] != OK {
			t.Fatalf("%s = %q em %+v", name, got[name], checks)
		}
	}
}

func TestDoctorNamesWhatIsWrong(t *testing.T) {
	manager, cfg, runner := setup(t, "linux")
	server := healthy(t)
	cfg.API = server.URL
	if _, err := manager.Install(context.Background(), cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	manager.Exe = "/somewhere/else/ai-usage"
	runner.answers["systemctl --user is-active ai-usage.service"] = "failed"
	runner.answers["loginctl show-user 1000 -p Linger --value"] = "no"
	items := []spool.Item{{Kind: spool.KindSample, State: spool.StateBlocked, Body: []byte(`{"provider":"claude"}`)}}
	if err := spool.Save(cfg.Spool, items); err != nil {
		t.Fatal(err)
	}
	checks := manager.Doctor(context.Background(), DoctorInput{Config: cfg, ConfigPath: "agent.json"})
	got := statuses(checks)
	if got["binary"] != Warn || got["service"] != Fail || got["linger"] != Warn || got["queue"] != Fail || got["codex"] != Warn {
		t.Fatalf("checks %+v", checks)
	}
	if !Failed(checks) {
		t.Fatal("deveria falhar")
	}
	for _, check := range checks {
		if check.Status != OK && check.Fix == "" && check.Name != "queue" {
			t.Fatalf("%s sem correção", check.Name)
		}
	}
}

func TestDoctorWithoutInstallOrConfig(t *testing.T) {
	manager, _, _ := setup(t, "linux")
	checks := manager.Doctor(context.Background(), DoctorInput{ConfigErr: os.ErrNotExist, ConfigPath: "agent.json"})
	got := statuses(checks)
	if got["config"] != Fail || got["service file"] != Fail || !Failed(checks) {
		t.Fatalf("checks %+v", checks)
	}
}

func TestDoctorReportsAnUnreachableServer(t *testing.T) {
	manager, cfg, _ := setup(t, "linux")
	cfg.API = "http://127.0.0.1:1"
	checks := manager.Doctor(context.Background(), DoctorInput{Config: cfg, ConfigPath: "agent.json"})
	if statuses(checks)["server"] != Fail {
		t.Fatalf("checks %+v", checks)
	}
}

func TestDoctorReadsLaunchdState(t *testing.T) {
	manager, cfg, runner := setup(t, "darwin")
	if _, err := manager.Install(context.Background(), cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	runner.answers["launchctl print gui/1000/dev.ai-usage.agent"] = "state = waiting\n"
	checks := manager.Doctor(context.Background(), DoctorInput{Config: cfg, ConfigPath: "agent.json"})
	if statuses(checks)["service"] != Fail {
		t.Fatalf("checks %+v", checks)
	}
	runner.answers["launchctl print gui/1000/dev.ai-usage.agent"] = "state = running\n"
	checks = manager.Doctor(context.Background(), DoctorInput{Config: cfg, ConfigPath: "agent.json"})
	if statuses(checks)["service"] != OK {
		t.Fatalf("checks %+v", checks)
	}
}
