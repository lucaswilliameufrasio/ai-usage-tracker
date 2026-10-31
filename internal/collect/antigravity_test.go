package collect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
)

func TestAntigravityKeepsPlanWindowsOnly(t *testing.T) {
	t.Setenv("TRACKER_VAULT_KEY", "canary-vault-key")
	t.Setenv("OPENAI_API_KEY", "canary-openai-key")
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.txt")
	argsFile := filepath.Join(dir, "args.txt")
	body := `{"status":"SUCCESS","num_turns":0,"response":"canary@example.com 80%","usage":{"input_tokens":9},"command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"gemini-weekly","window":"weekly","remaining_fraction":0.5,"reset_time":"2026-11-06T15:00:00Z","account_email":"canary@example.com"},{"id":"gemini-5h","window":"5h","remainingFraction":0.8,"resetTime":"2026-10-30T20:00:00Z"},{"id":"gemini-argon-weekly","window":"weekly","remaining_fraction":1,"reset_time":"2026-11-06T15:00:00Z"}]},{"name":"Claude and GPT models","buckets":[{"id":"3p-weekly","window":"weekly","remaining_fraction":0.2,"reset_time":"2026-11-06T15:00:00Z"}]}]}}}`
	script := filepath.Join(dir, "agy")
	writeScript(t, script, `#!/bin/sh
envfile=`+shellQuote(t, envFile)+`
argsfile=`+shellQuote(t, argsFile)+`
if [ -n "${TRACKER_VAULT_KEY:-}" ]; then printf 'vault\n' >> "$envfile"; fi
if [ -n "${OPENAI_API_KEY:-}" ]; then printf 'openai\n' >> "$envfile"; fi
if [ -n "${HOME:-}" ]; then printf 'home\n' >> "$envfile"; fi
if [ "${AGY_CLI_DISABLE_AUTO_UPDATE:-}" = "true" ]; then printf 'noupdate\n' >> "$envfile"; fi
if [ "${BROWSER:-}" = "/bin/false" ]; then printf 'browser\n' >> "$envfile"; fi
printf '%s\n' "$@" > "$argsfile"
printf '%s\n' '`+body+`'
`)
	now := time.Date(2026, 10, 30, 18, 4, 5, 123, time.UTC)
	sample, err := ReadAntigravity(context.Background(), script, now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sample.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "canary@example.com") || strings.Contains(string(raw), "80%") {
		t.Fatal("a amostra ficou com texto da resposta")
	}
	decoded, err := contract.DecodeSample(raw, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Provider != contract.ProviderAntigravity || decoded.Plan != "" {
		t.Fatalf("provedor %s plano %s", decoded.Provider, decoded.Plan)
	}
	want := map[string]float64{
		"gemini_seven_day":       50,
		"gemini_five_hour":       20,
		"gemini_argon_seven_day": 0,
		"claude_gpt_seven_day":   80,
	}
	if len(decoded.Windows) != len(want) {
		t.Fatalf("janelas %#v", decoded.Windows)
	}
	for _, window := range decoded.Windows {
		used, ok := want[window.Name]
		if !ok || window.Used != used || window.Limit != 100 || window.Unit != contract.UnitPercent {
			t.Fatalf("janela %#v", window)
		}
	}
	envRaw, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	envText := string(envRaw)
	if strings.Contains(envText, "vault") || strings.Contains(envText, "openai") || !strings.Contains(envText, "home") || !strings.Contains(envText, "noupdate") || !strings.Contains(envText, "browser") {
		t.Fatalf("ambiente %s", envText)
	}
	argsRaw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(argsRaw), "/usage") || !strings.Contains(string(argsRaw), "json") {
		t.Fatalf("args %s", argsRaw)
	}
}

func TestAntigravityIgnoresAModelCall(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agy")
	writeScript(t, script, `#!/bin/sh
printf '%s\n' '{"status":"SUCCESS","num_turns":1,"response":"ok","usage":{"input_tokens":10,"output_tokens":2}}'
`)
	_, err := ReadAntigravity(context.Background(), script, time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("conversa virou janela")
	}
}

func TestAntigravityFailureDoesNotInventAWindow(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agy")
	writeScript(t, script, "#!/bin/sh\nexit 1\n")
	_, err := ReadAntigravity(context.Background(), script, time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("falha virou amostra")
	}
}
