package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ai-usage-tracker/internal/agent"
)

func TestWizardAcceptsSuggestionsWithoutRepeatingSecrets(t *testing.T) {
	home := t.TempDir()
	observed := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	claude := `{"cachedUsageUtilization":{"fetchedAtMs":` + strconv.FormatInt(observed.UnixMilli(), 10) + `,"utilization":{"five_hour":{"utilization":41,"resets_at":"2026-10-29T18:00:00Z"}}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(claude), 0o600); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(home, ".codex", "sessions", "2026", "09", "20")
	if err := os.MkdirAll(older, 0o700); err != nil {
		t.Fatal(err)
	}
	limits := `{"timestamp":"2026-09-20T15:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"plan_type":"pro","primary":{"used_percent":25,"window_minutes":300,"resets_at":1761751800}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(older, "rollout-old.jsonl"), []byte(limits), 0o600); err != nil {
		t.Fatal(err)
	}
	var screen strings.Builder
	input := strings.NewReader("http://127.0.0.1:8787\n\ncodigo-teste\n\n\n\n")
	file, code, name, err := runWizard(wizardOptions{
		in:        input,
		out:       &screen,
		home:      home,
		now:       time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		tokenPath: filepath.Join(home, "device.token"),
		spoolPath: filepath.Join(home, "spool.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if file.API != "http://127.0.0.1:8787" || file.ClaudeFile != filepath.Join(home, ".claude.json") || file.CodexDir != filepath.Join(home, ".codex", "sessions") {
		t.Fatalf("%+v", file)
	}
	if code != "codigo-teste" || name != "notebook" {
		t.Fatalf("code %s name %s", code, name)
	}
	shown := screen.String()
	if strings.Contains(shown, "codigo-teste") || strings.Contains(shown, "41") || strings.Contains(shown, "25") {
		t.Fatal("a tela repetiu código ou percentual")
	}
	if !strings.Contains(shown, "20/09/2026 15:00 UTC") {
		t.Fatal(shown)
	}
}

func TestChooseRunKeepsFlagsAndSavedConfig(t *testing.T) {
	saved := agent.File{API: "http://127.0.0.1:8787", TokenFile: "device.token", Spool: "spool.json", CodexDir: "sessions"}
	if chooseRun(true, saved, true) != modeFlags {
		t.Fatal("flags")
	}
	if chooseRun(false, saved, false) != modeExecute {
		t.Fatal("config")
	}
	if chooseRun(false, agent.File{}, false) != modeNeedTerminal {
		t.Fatal("pipe")
	}
	if chooseRun(false, agent.File{}, true) != modeWizard {
		t.Fatal("setup")
	}
}
