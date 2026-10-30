package collect

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
)

func TestSyntheticFixturesOmitPrivateFields(t *testing.T) {
	root := filepath.Join("..", "..", "fixtures", "synthetic")
	now := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	batch, err := Read(Request{
		ClaudeFile: filepath.Join(root, "claude-cache.json"),
		CodexDir:   filepath.Join(root, "codex"),
		Now:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Samples) != 2 || len(batch.Reports) != 0 {
		t.Fatalf("samples %d reports %d", len(batch.Samples), len(batch.Reports))
	}
	raw, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"CANARY_PROMPT", "CANARY_TOKEN", "acct_canary_uuid", "/home/canary/secret-project"} {
		if strings.Contains(string(raw), canary) {
			t.Fatalf("canário vazou: %s", canary)
		}
	}
	byProvider := map[string]int{}
	for _, sample := range batch.Samples {
		byProvider[sample.Provider] = len(sample.Windows)
		if sample.SampleID == "" {
			t.Fatal("amostra sem id")
		}
	}
	if byProvider["claude"] != 2 || byProvider["codex"] != 2 {
		t.Fatalf("janelas %#v", byProvider)
	}
}

func TestStatuslineAndMissingLimits(t *testing.T) {
	root := filepath.Join("..", "..", "fixtures", "synthetic")
	now := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	batch, err := Read(Request{ClaudeFile: filepath.Join(root, "claude-statusline.json"), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Samples) != 1 || batch.Samples[0].Windows[0].Used != 23.5 {
		t.Fatalf("%#v", batch.Samples)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.json"), []byte(`{"cachedUsageUtilization":{"utilization":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	batch, err = Read(Request{ClaudeFile: filepath.Join(dir, "empty.json"), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Reports) != 1 || batch.Reports[0].ReasonCode != "LIMITS_ABSENT" {
		t.Fatalf("%#v", batch.Reports)
	}
}

func TestSameSnapshotKeepsIdentity(t *testing.T) {
	root := filepath.Join("..", "..", "fixtures", "synthetic")
	now := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	first, err := Read(Request{ClaudeFile: filepath.Join(root, "claude-cache.json"), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Read(Request{ClaudeFile: filepath.Join(root, "claude-cache.json"), Now: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Samples[0].SampleID != second.Samples[0].SampleID {
		t.Fatal("releitura do mesmo cache mudou o id")
	}
}

func TestCodexKeepsOlderLimitsWhenANewerSessionHasNone(t *testing.T) {
	root := t.TempDir()
	older := filepath.Join(root, "2026", "09", "20")
	newer := filepath.Join(root, "2026", "10", "28")
	if err := os.MkdirAll(older, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newer, 0o700); err != nil {
		t.Fatal(err)
	}
	limits := []byte(`{"timestamp":"2026-09-20T15:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"plan_type":"pro","primary":{"used_percent":25,"window_minutes":300,"resets_at":1761751800},"secondary":{"used_percent":12,"window_minutes":10080,"resets_at":1762354800}}}}` + "\n")
	if err := os.WriteFile(filepath.Join(older, "rollout-old.jsonl"), limits, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newer, "rollout-new.jsonl"), []byte(`{"timestamp":"2026-10-28T12:00:00Z","type":"session_meta","payload":{"type":"session_meta"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "auth.json"), []byte("CANARY_TOKEN"), 0o600); err != nil {
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
	batch, err := Read(Request{CodexDir: root, Now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Samples) != 1 || len(batch.Reports) != 0 {
		t.Fatalf("samples %d reports %d", len(batch.Samples), len(batch.Reports))
	}
	observed := batch.Samples[0].SourceObservedAt
	if observed == nil || !observed.Equal(oldTime) {
		t.Fatal("a sessão nova sem limite escondeu a observação anterior")
	}
	raw, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "CANARY_TOKEN") {
		t.Fatal("arquivo ao lado do rollout entrou na amostra")
	}
}

func TestOversizedRolloutLineIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	line := append([]byte(`{"rate_limits":{"plan_type":"pro"},"pad":"`), bytes.Repeat([]byte("a"), (2<<20)+8)...)
	line = append(line, []byte("\"}\n")...)
	if err := os.WriteFile(filepath.Join(dir, "rollout-huge.jsonl"), line, 0o600); err != nil {
		t.Fatal(err)
	}
	batch, err := Read(Request{CodexDir: dir, Now: time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Samples) != 0 || len(batch.Reports) != 1 {
		t.Fatalf("samples %d reports %d", len(batch.Samples), len(batch.Reports))
	}
	if batch.Reports[0].ReasonCode != contract.ReasonSourceUnreadable {
		t.Fatalf("reason %s", batch.Reports[0].ReasonCode)
	}
}
