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
