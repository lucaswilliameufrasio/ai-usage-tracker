package journey_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ai-usage-tracker/internal/agent"
	"ai-usage-tracker/internal/httpapi"
	"ai-usage-tracker/internal/push"
	"ai-usage-tracker/internal/store"
)

func TestPersonalJourney(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var logs bytes.Buffer
	sender := push.NewRecording("BEl0test-public-key")
	clock := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	var posts atomic.Int32
	api := &httpapi.Server{
		Store:          db,
		Now:            func() time.Time { return clock },
		Sender:         sender,
		AllowOpenSetup: true,
		Logger:         slog.New(slog.NewTextHandler(&logs, nil)),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/samples" || r.URL.Path == "/v1/source-reports" {
			posts.Add(1)
		}
		api.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	operator := setup(t, server.URL)
	code := enrollment(t, server.URL, operator)
	enrolled, err := agent.Enroll(context.Background(), server.Client(), server.URL, code, "notebook")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.Enroll(context.Background(), server.Client(), server.URL, code, "outro"); err == nil {
		t.Fatal("código reutilizado")
	}
	subscribe(t, server.URL, operator)
	source := filepath.Join(dir, "sources")
	if err := os.MkdirAll(filepath.Join(source, "codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeClaude(t, filepath.Join(source, "claude.json"), clock, 86)
	writeCodex(t, filepath.Join(source, "codex", "rollout-one.jsonl"), clock, 25)
	spoolPath := filepath.Join(dir, "spool.json")
	first, err := agent.Run(context.Background(), agent.Config{
		API: server.URL, Token: enrolled.Token, SpoolPath: spoolPath,
		ClaudeFile: filepath.Join(source, "claude.json"), CodexDir: filepath.Join(source, "codex"), Now: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Accepted != 2 {
		t.Fatalf("aceitas %d", first.Accepted)
	}
	payloads, err := db.StoredPayloads()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(payloads, "\n")
	for _, canary := range []string{"CANARY_PROMPT", "CANARY_TOKEN", "/home/canary/secret-project", "acct_canary_uuid"} {
		if strings.Contains(joined, canary) || strings.Contains(logs.String(), canary) {
			t.Fatalf("dado privado persistido ou logado: %s", canary)
		}
	}
	count, err := db.CountSamples()
	if err != nil || count != 2 {
		t.Fatalf("amostras %d %v", count, err)
	}
	second, err := agent.Run(context.Background(), agent.Config{
		API: server.URL, Token: enrolled.Token, SpoolPath: spoolPath,
		ClaudeFile: filepath.Join(source, "claude.json"), CodexDir: filepath.Join(source, "codex"), Now: clock.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Duplicate < 2 {
		t.Fatalf("duplicatas %d", second.Duplicate)
	}
	count, _ = db.CountSamples()
	if count != 2 {
		t.Fatalf("histórico duplicou: %d", count)
	}
	overview := getJSON(t, server.URL+"/v1/overview", operator)
	providers := overviewProviders(t, overview)
	seen := map[string]string{}
	for _, item := range providers {
		card := item.(map[string]any)
		seen[card["provider"].(string)] = card["freshness"].(string)
	}
	if seen["claude"] != "current" || seen["codex"] != "current" {
		t.Fatalf("frescor %#v", seen)
	}
	if len(sender.Payloads()) == 0 {
		t.Fatal("limiar não gerou push")
	}
	pushes := len(sender.Payloads())
	_, _ = agent.Run(context.Background(), agent.Config{
		API: server.URL, Token: enrolled.Token, SpoolPath: spoolPath,
		ClaudeFile: filepath.Join(source, "claude.json"), CodexDir: filepath.Join(source, "codex"), Now: clock.Add(2 * time.Minute),
	})
	if len(sender.Payloads()) != pushes {
		t.Fatal("alerta duplicado")
	}

	clock = clock.Add(2 * time.Hour)
	stale := getJSON(t, server.URL+"/v1/overview", operator)
	for _, item := range overviewProviders(t, stale) {
		card := item.(map[string]any)
		if card["freshness"] != "stale" {
			t.Fatalf("esperado desatualizado, veio %v", card["freshness"])
		}
		if card["observed_at"] == nil {
			t.Fatal("desatualizado sem instante")
		}
	}

	writeClaude(t, filepath.Join(source, "claude.json"), clock, 0)
	emptyDir := filepath.Join(dir, "empty-codex")
	if err := os.MkdirAll(emptyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyDir, "rollout-empty.jsonl"), []byte("{\"payload\":{\"rate_limits\":null}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.Run(context.Background(), agent.Config{
		API: server.URL, Token: enrolled.Token, SpoolPath: filepath.Join(dir, "spool-unavailable.json"),
		ClaudeFile: filepath.Join(source, "missing-claude.json"), CodexDir: emptyDir, Now: clock,
	}); err != nil {
		t.Fatal(err)
	}
	unavailable := getJSON(t, server.URL+"/v1/overview", operator)
	for _, item := range overviewProviders(t, unavailable) {
		card := item.(map[string]any)
		if card["provider"] == "codex" && card["freshness"] != "unavailable" {
			t.Fatalf("codex %#v", card)
		}
	}

	revoke(t, server.URL, operator, enrolled.DeviceID)
	before := posts.Load()
	writeClaude(t, filepath.Join(source, "claude.json"), clock, 91)
	revoked, err := agent.Run(context.Background(), agent.Config{
		API: server.URL, Token: enrolled.Token, SpoolPath: spoolPath,
		ClaudeFile: filepath.Join(source, "claude.json"), CodexDir: filepath.Join(source, "codex"), Now: clock,
	})
	if err != nil || !revoked.Blocked {
		t.Fatalf("revogação não bloqueou: %+v %v", revoked, err)
	}
	if posts.Load() != before+1 {
		t.Fatalf("envios após revogação %d -> %d", before, posts.Load())
	}
	blockedAgain, err := agent.Run(context.Background(), agent.Config{
		API: server.URL, Token: enrolled.Token, SpoolPath: spoolPath,
		ClaudeFile: filepath.Join(source, "claude.json"), CodexDir: filepath.Join(source, "codex"), Now: clock.Add(time.Minute),
	})
	if err != nil || !blockedAgain.Blocked {
		t.Fatal(err)
	}
	if posts.Load() != before+1 {
		t.Fatal("fila tentou reativar credencial revogada")
	}
	spoolRaw, err := os.ReadFile(spoolPath)
	if err != nil || !strings.Contains(string(spoolRaw), "blocked_revoked") {
		t.Fatal("fila não preservou item recusado")
	}
	history := getJSON(t, server.URL+"/v1/samples?limit=10", operator)
	if history["has_more"].(bool) {
		t.Fatal("has_more inesperado")
	}
	if len(history["data"].([]any)) != countFrom(t, db) {
		t.Fatal("histórico não bate com o sqlite")
	}
}

func TestQuietHoursSuppressPush(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sender := push.NewRecording("BEl0test-public-key")
	clock := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	api := &httpapi.Server{Store: db, Now: func() time.Time { return clock }, Sender: sender, AllowOpenSetup: true}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)
	operator := setup(t, server.URL)
	putJSON(t, server.URL+"/v1/settings", operator, map[string]any{
		"stale_after_seconds":     1800,
		"alert_threshold_percent": 80,
		"timezone":                "UTC",
		"quiet_hours":             map[string]string{"start": "14:00", "end": "16:00"},
	})
	code := enrollment(t, server.URL, operator)
	enrolled, err := agent.Enroll(context.Background(), server.Client(), server.URL, code, "notebook")
	if err != nil {
		t.Fatal(err)
	}
	subscribe(t, server.URL, operator)
	source := filepath.Join(dir, "claude.json")
	writeClaude(t, source, clock, 90)
	if _, err := agent.Run(context.Background(), agent.Config{
		API: server.URL, Token: enrolled.Token, SpoolPath: filepath.Join(dir, "spool.json"),
		ClaudeFile: source, Now: clock,
	}); err != nil {
		t.Fatal(err)
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("horário silencioso enviou push")
	}
	deliveries := getJSON(t, server.URL+"/v1/push/deliveries", operator)
	found := false
	for _, item := range deliveries["data"].([]any) {
		if item.(map[string]any)["status"] == "suppressed" {
			found = true
		}
	}
	if !found {
		t.Fatal("supressão não ficou visível")
	}
}

func TestDeviceCannotReadAndUnknownFieldIsRejected(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	api := &httpapi.Server{Store: db, Now: time.Now, Sender: push.NewRecording("key"), AllowOpenSetup: true}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)
	operator := setup(t, server.URL)
	code := enrollment(t, server.URL, operator)
	enrolled, err := agent.Enroll(context.Background(), server.Client(), server.URL, code, "notebook")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/overview", nil)
	req.Header.Set("Authorization", "Bearer "+enrolled.Token)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body := `{"sample_id":"smp_abcdef12","provider":"claude","collected_at":"` + time.Now().UTC().Format(time.RFC3339) + `","prompt":"CANARY_PROMPT","windows":[{"name":"five_hour","used":1,"limit":100,"unit":"percent","resets_at":"2026-10-29T20:00:00Z"}]}`
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/samples", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+enrolled.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(raw), "INVALID_PARAMS") {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	payloads, _ := db.StoredPayloads()
	if strings.Contains(strings.Join(payloads, "\n"), "CANARY_PROMPT") {
		t.Fatal("prompt recusado foi gravado")
	}
}

func setup(t *testing.T, api string) string {
	t.Helper()
	resp, err := http.Post(api+"/v1/setup", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated || body["operator_token"] == "" {
		t.Fatalf("setup %d %#v", resp.StatusCode, body)
	}
	return body["operator_token"]
}

func enrollment(t *testing.T, api string, operator string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, api+"/v1/enrollment-codes", nil)
	req.Header.Set("Authorization", "Bearer "+operator)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	code, _ := body["code"].(string)
	if resp.StatusCode != http.StatusCreated || code == "" {
		t.Fatalf("código %d %#v", resp.StatusCode, body)
	}
	return code
}

func subscribe(t *testing.T, api string, operator string) {
	t.Helper()
	putJSON(t, api+"/v1/push/subscriptions", operator, map[string]string{
		"endpoint": "https://push.example.test/subscription/one",
		"p256dh":   "BEl0-test-key-value",
		"auth":     "auth-test-value",
	})
}

func revoke(t *testing.T, api string, operator string, id string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, api+"/v1/devices/"+id+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+operator)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revogação %d", resp.StatusCode)
	}
}

func writeClaude(t *testing.T, path string, observed time.Time, used int) {
	t.Helper()
	payload := map[string]any{
		"prompt":          "CANARY_PROMPT",
		"transcript_path": "/home/canary/secret-project/file.txt",
		"oauthAccount":    map[string]string{"access_token": "CANARY_TOKEN"},
		"cachedUsageUtilization": map[string]any{
			"accountUuid": "acct_canary_uuid",
			"fetchedAtMs": observed.UnixMilli(),
			"utilization": map[string]any{
				"five_hour": map[string]any{"utilization": used, "resets_at": observed.Add(30 * time.Minute).Format(time.RFC3339)},
				"seven_day": map[string]any{"utilization": 10, "resets_at": observed.Add(7 * 24 * time.Hour).Format(time.RFC3339)},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeCodex(t *testing.T, path string, observed time.Time, used int) {
	t.Helper()
	line := map[string]any{
		"timestamp": observed.Format(time.RFC3339),
		"payload": map[string]any{
			"type": "token_count",
			"rate_limits": map[string]any{
				"plan_type": "pro",
				"primary": map[string]any{
					"used_percent":   used,
					"window_minutes": 300,
					"resets_at":      observed.Add(30 * time.Minute).Unix(),
				},
				"secondary": map[string]any{
					"used_percent":   12,
					"window_minutes": 10080,
					"resets_at":      observed.Add(7 * 24 * time.Hour).Unix(),
				},
			},
			"prompt": "CANARY_PROMPT",
		},
	}
	raw, _ := json.Marshal(line)
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func overviewProviders(t *testing.T, overview map[string]any) []any {
	t.Helper()
	if accounts, ok := overview["accounts"].([]any); ok && len(accounts) > 0 {
		group, ok := accounts[0].(map[string]any)
		if !ok {
			t.Fatal("grupo ilegível")
		}
		providers, ok := group["providers"].([]any)
		if !ok {
			t.Fatal("grupo sem provedores")
		}
		return providers
	}
	providers, ok := overview["providers"].([]any)
	if !ok {
		t.Fatal("visão sem provedores")
	}
	return providers
}

func getJSON(t *testing.T, url string, operator string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+operator)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %d %#v", url, resp.StatusCode, body)
	}
	return body
}

func putJSON(t *testing.T, url string, operator string, payload any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(raw))
	if strings.Contains(url, "/push/subscriptions") {
		req, _ = http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	}
	req.Header.Set("Authorization", "Bearer "+operator)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %d %s", url, resp.StatusCode, raw)
	}
}

func countFrom(t *testing.T, db *store.Store) int {
	t.Helper()
	count, err := db.CountSamples()
	if err != nil {
		t.Fatal(err)
	}
	return count
}
