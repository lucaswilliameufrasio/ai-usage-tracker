package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
)

func TestLiveCodexReplacesTheFileSnapshot(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	writeRollout(t, dir, 7)
	script := filepath.Join(dir, "codex")
	writeAgentScript(t, script, `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    *'"method":"account/rateLimits/read"'*)
      printf '%s\n' '{"id":2,"result":{"email":"canary@example.com","accountId":"CANARY_ACCOUNT","rateLimits":{"planType":"plus","primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":1761751800},"secondary":{"usedPercent":12,"windowDurationMins":10080,"resetsAt":1762354800}}}}'
      ;;
  esac
done
`)
	bodies := serveSamples(t)
	result, err := Run(context.Background(), Config{
		API: bodies.URL, Token: "device-token", SpoolPath: filepath.Join(dir, "spool.json"),
		CodexDir: dir, CodexCommand: script, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CodexLive != "read" || result.Accepted != 1 {
		t.Fatalf("resultado %+v", result)
	}
	sample := decodePosted(t, bodies.snapshot()[0], now.Add(time.Minute))
	if sample.Plan != "plus" || sample.SourceObservedAt == nil || !sample.SourceObservedAt.Equal(now) {
		t.Fatalf("observado %v plano %s", sample.SourceObservedAt, sample.Plan)
	}
	for _, window := range sample.Windows {
		if window.Name == "five_hour" && window.Used == 7 {
			t.Fatal("a amostra do arquivo substituiu a leitura ao vivo")
		}
	}
	for _, canary := range []string{"canary@example.com", "CANARY_ACCOUNT"} {
		if strings.Contains(string(bodies.snapshot()[0]), canary) {
			t.Fatalf("o envio ficou com %s", canary)
		}
	}
}

func TestLiveCodexFailureKeepsTheFileSnapshot(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	writeRollout(t, dir, 7)
	script := filepath.Join(dir, "codex")
	writeAgentScript(t, script, "#!/bin/sh\nexit 1\n")
	bodies := serveSamples(t)
	result, err := Run(context.Background(), Config{
		API: bodies.URL, Token: "device-token", SpoolPath: filepath.Join(dir, "spool.json"),
		CodexDir: dir, CodexCommand: script, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CodexLive != "fallback" || result.Accepted != 1 {
		t.Fatalf("resultado %+v", result)
	}
	sample := decodePosted(t, bodies.snapshot()[0], now.Add(time.Minute))
	if sample.SourceObservedAt == nil || sample.SourceObservedAt.Equal(now) {
		t.Fatal("a falha virou observação nova")
	}
	found := false
	for _, window := range sample.Windows {
		if window.Name == "five_hour" && window.Used == 7 {
			found = true
		}
	}
	if !found {
		t.Fatal("a observação do arquivo não foi enviada")
	}
}

type postedBodies struct {
	URL   string
	mu    sync.Mutex
	items [][]byte
}

func (p *postedBodies) snapshot() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.items))
	copy(out, p.items)
	return out
}

func serveSamples(t *testing.T) *postedBodies {
	t.Helper()
	posted := &postedBodies{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			t.Errorf("ler corpo: %v", err)
			http.Error(w, "erro", http.StatusBadRequest)
			return
		}
		posted.mu.Lock()
		posted.items = append(posted.items, raw)
		posted.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)
	posted.URL = server.URL
	return posted
}

func writeRollout(t *testing.T, dir string, used int) {
	t.Helper()
	line := `{"timestamp":"2026-09-20T15:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"plan_type":"pro","primary":{"used_percent":` + itoa(used) + `,"window_minutes":300,"resets_at":1761751800}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rollout-old.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeAgentScript(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func decodePosted(t *testing.T, raw []byte, now time.Time) contract.Sample {
	t.Helper()
	sample, err := contract.DecodeSample(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	return sample
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func TestAntigravityPostsTheWindowWithoutTheReply(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	script := filepath.Join(dir, "agy")
	writeAgentScript(t, script, `#!/bin/sh
printf '%s\n' '{"status":"SUCCESS","num_turns":0,"response":"canary@example.com","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"gemini-weekly","window":"weekly","remaining_fraction":0.5,"reset_time":"2026-11-06T15:00:00Z"}]}]}}}'
`)
	bodies := serveSamples(t)
	result, err := Run(context.Background(), Config{
		API: bodies.URL, Token: "device-token", SpoolPath: filepath.Join(dir, "spool.json"),
		AgyCommand: script, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AgyLive != "read" || result.Accepted != 1 {
		t.Fatalf("resultado %+v", result)
	}
	sample := decodePosted(t, bodies.snapshot()[0], now.Add(time.Minute))
	if sample.Provider != contract.ProviderAntigravity || len(sample.Windows) != 1 || sample.Windows[0].Name != "gemini_seven_day" || sample.Windows[0].Used != 50 {
		t.Fatalf("%+v", sample)
	}
	if strings.Contains(string(bodies.snapshot()[0]), "canary@example.com") {
		t.Fatal("o envio ficou com o texto da resposta")
	}
}
