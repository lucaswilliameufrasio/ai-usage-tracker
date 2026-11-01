package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSuggestHidesUsageAndKeepsTheLastCodexLimit(t *testing.T) {
	home := t.TempDir()
	observed := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	claude := `{"cachedUsageUtilization":{"fetchedAtMs":` + strconv.FormatInt(observed.UnixMilli(), 10) + `,"utilization":{"five_hour":{"utilization":41,"resets_at":"2026-10-29T18:00:00Z"}}},"oauth":"CANARY_TOKEN"}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(claude), 0o600); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(home, ".codex", "sessions", "2026", "09", "20")
	newer := filepath.Join(home, ".codex", "sessions", "2026", "10", "28")
	if err := os.MkdirAll(older, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newer, 0o700); err != nil {
		t.Fatal(err)
	}
	limits := `{"timestamp":"2026-09-20T15:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"plan_type":"pro","primary":{"used_percent":25,"window_minutes":300,"resets_at":1761751800}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(older, "rollout-old.jsonl"), []byte(limits), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newer, "rollout-new.jsonl"), []byte("{\"timestamp\":\"2026-10-28T12:00:00Z\",\"type\":\"session_meta\",\"payload\":{\"cwd\":\"/home/canary/secret-project\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(older, "rollout-old.jsonl"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(newer, "rollout-new.jsonl"), newTime, newTime); err != nil {
		t.Fatal(err)
	}
	claudePlace, codexPlace := Suggest(home, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	if claudePlace.Path != filepath.Join(home, ".claude.json") {
		t.Fatal("claude")
	}
	if codexPlace.Path != filepath.Join(home, ".codex", "sessions") {
		t.Fatal("codex")
	}
	joined := claudePlace.Detail + "\n" + codexPlace.Detail
	claudeWhen := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC).In(time.Local).Format("2006-01-02 15:04 MST")
	codexWhen := time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC).In(time.Local).Format("2006-01-02 15:04 MST")
	if !strings.Contains(claudePlace.Detail, claudeWhen) {
		t.Fatal(claudePlace.Detail)
	}
	if !strings.Contains(codexPlace.Detail, codexWhen) || !strings.Contains(codexPlace.Detail, "Newer sessions carried no rate_limits.") {
		t.Fatal(codexPlace.Detail)
	}
	for _, hidden := range []string{"41", "25", "CANARY_TOKEN", "secret-project"} {
		if strings.Contains(joined, hidden) {
			t.Fatalf("detalhe expôs %s", hidden)
		}
	}
}
