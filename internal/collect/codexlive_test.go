package collect

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
)

func TestCodexLiveKeepsOnlyLimits(t *testing.T) {
	t.Setenv("TRACKER_VAULT_KEY", "canary-vault-key")
	t.Setenv("OPENAI_API_KEY", "canary-openai-key")
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.txt")
	replyFile := filepath.Join(dir, "reply.txt")
	result := `{"id":2,"result":{"accountId":"CANARY_ACCOUNT","email":"canary@example.com","rateLimitResetCredits":{"credits":[{"title":"CANARY_SECRET_TITLE"}]},"rateLimits":{"planType":"Plus","limitId":"CANARY_LIMIT","primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":1761751800},"secondary":{"usedPercent":12,"windowDurationMins":10080,"resetsAt":1762354800}}}}`
	script := filepath.Join(dir, "codex")
	writeScript(t, script, `#!/bin/sh
envfile=`+shellQuote(t, envFile)+`
replyfile=`+shellQuote(t, replyFile)+`
if [ -n "${TRACKER_VAULT_KEY:-}" ]; then printf 'vault\n' >> "$envfile"; fi
if [ -n "${OPENAI_API_KEY:-}" ]; then printf 'openai\n' >> "$envfile"; fi
if [ -n "${HOME:-}" ]; then printf 'home\n' >> "$envfile"; fi
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"id":1,"result":{"userAgent":"fake"}}'
      ;;
    *'"method":"account/rateLimits/read"'*)
      printf '%s\n' '{"id":9,"method":"account/chatgptAuthTokens/refresh","params":{"token":"CANARY_TOKEN"}}'
      IFS= read -r reply
      printf '%s\n' "$reply" > "$replyfile"
      printf '%s\n' '`+result+`'
      ;;
  esac
done
`)
	now := time.Date(2026, 10, 30, 18, 4, 5, 123, time.UTC)
	sample, err := ReadCodexLive(context.Background(), script, now)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Plan != "plus" || sample.SourceObservedAt == nil || !sample.SourceObservedAt.Equal(now.Truncate(time.Second)) {
		t.Fatalf("plano %s observado %v", sample.Plan, sample.SourceObservedAt)
	}
	seen := map[string]float64{}
	for _, window := range sample.Windows {
		seen[window.Name] = window.Used
	}
	if seen["five_hour"] != 25 || seen["seven_day"] != 12 {
		t.Fatalf("janelas %#v", seen)
	}
	raw, err := sample.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"CANARY_ACCOUNT", "canary@example.com", "CANARY_SECRET_TITLE", "CANARY_LIMIT", "CANARY_TOKEN", "canary-vault-key", "canary-openai-key"} {
		if strings.Contains(string(raw), canary) {
			t.Fatalf("a amostra ficou com %s", canary)
		}
	}
	envText, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(envText, []byte("home")) || bytes.Contains(envText, []byte("vault")) || bytes.Contains(envText, []byte("openai")) {
		t.Fatal("o ambiente do processo filho não ficou restrito")
	}
	reply, err := os.ReadFile(replyFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reply, []byte("CANARY_TOKEN")) || !bytes.Contains(reply, []byte("method not found")) {
		t.Fatal("a resposta repetiu o pedido do servidor")
	}
}

func TestCodexLiveFailureIsGeneric(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "codex")
	writeScript(t, script, "#!/bin/sh\nprintf '%s\n' 'CANARY_TOKEN' >&2\nexit 1\n")
	_, err := ReadCodexLive(context.Background(), script, time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC))
	if err == nil || err.Error() != "leitura ao vivo do Codex falhou" || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("erro %v", err)
	}
}

func TestCodexLiveKillsAStuckProcess(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "codex")
	writeScript(t, script, "#!/bin/sh\nsleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := ReadCodexLive(ctx, script, time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("esperado erro")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("o prazo não encerrou a leitura")
	}
	out, psErr := exec.Command("ps", "-eo", "args").Output()
	if psErr == nil && bytes.Contains(out, []byte(script)) {
		t.Fatal("o processo falso continuou depois do prazo")
	}
}

func TestSampleFromRateLimitsRejectsAnEmptyWindow(t *testing.T) {
	now := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	_, err := sampleFromRateLimits([]byte(`{"email":"canary@example.com","rateLimits":{"primary":null}}`), now)
	if err == nil {
		t.Fatal("janela vazia")
	}
}

func TestPreferCodexDropsTheFileSnapshotAndReport(t *testing.T) {
	batch := Batch{
		Samples: []contract.Sample{{Provider: contract.ProviderCodex, Plan: "old"}, {Provider: contract.ProviderClaude, Plan: "keep"}},
		Reports: []contract.SourceReport{{Provider: contract.ProviderCodex}, {Provider: contract.ProviderClaude}},
	}
	next := PreferCodex(batch, contract.Sample{Provider: contract.ProviderCodex, Plan: "plus"})
	if len(next.Samples) != 2 || next.Samples[0].Provider != contract.ProviderClaude || next.Samples[1].Plan != "plus" {
		t.Fatalf("amostras %#v", next.Samples)
	}
	if len(next.Reports) != 1 || next.Reports[0].Provider != contract.ProviderClaude {
		t.Fatalf("relatórios %#v", next.Reports)
	}
}

func writeScript(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(t *testing.T, path string) string {
	t.Helper()
	if strings.Contains(path, "'") {
		t.Fatal("caminho inesperado")
	}
	return "'" + path + "'"
}
