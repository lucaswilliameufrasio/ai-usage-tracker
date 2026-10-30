package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-usage-tracker/internal/sabia"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

func TestAdminEditsSabiaTemplateWithoutReturningTheKey(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracker.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := vault.OpenOwn(filepath.Join(dir, "vault"), bytesRepeat())
	if err != nil {
		t.Fatal(err)
	}
	key := "sb_deploy_sintetico"
	var hits []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		hits = append(hits, r.URL.Path+" "+string(raw))
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"recusada ` + key + `"}`))
			return
		}
		switch r.URL.Path {
		case "/v1/template-deployments/plan":
			_, _ = w.Write([]byte(`{"changes":[{"id":"usage-alert-v1","operation":"create","confirmation_required":false}]}`))
		case "/v1/template-deployments/apply":
			_, _ = w.Write([]byte(`{"applied":1,"unchanged":0,"deleted":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)
	handler := (&Server{
		Store:          db,
		Vault:          box,
		AllowOpenSetup: true,
		SabiaHTTP:      fake.Client(),
		Now:            func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	operator := setupOperator(t, handler)
	draft := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin": fake.URL,
		"template": map[string]string{
			"name":    "Alerta de uso",
			"subject": "Uso de {{provider}} em {{used}}%",
			"html":    "<p>{{used}}</p>",
			"text":    "{{used}}",
		},
	})
	if draft.Code != http.StatusOK || strings.Contains(draft.Body, key) || !strings.Contains(draft.Body, `"deploy_key_present":false`) {
		t.Fatalf("rascunho %d vazou=%v", draft.Code, strings.Contains(draft.Body, key))
	}
	missing := callJSON(t, handler, http.MethodPost, "/v1/admin/sabia/plan", operator, nil)
	if missing.Code != http.StatusPreconditionFailed || strings.Contains(missing.Body, key) || !strings.Contains(missing.Body, "não está no cofre") {
		t.Fatalf("sem chave %d", missing.Code)
	}
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin":         fake.URL,
		"application_id": "11111111-1111-1111-1111-111111111111",
		"deploy_key":     key,
		"template": map[string]string{
			"name":    "Alerta de uso",
			"subject": "Uso de {{provider}} em {{used}}%",
			"html":    "<p>{{used}}</p>",
			"text":    "{{used}}",
		},
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, key) || !strings.Contains(saved.Body, `"deploy_key_present":true`) {
		t.Fatalf("gravação %d vazou=%v", saved.Code, strings.Contains(saved.Body, key))
	}
	got, err := box.Get(context.Background(), sabia.DeploySecret)
	if err != nil || string(got) != key {
		t.Fatal("cofre")
	}
	rawDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawDB), key) {
		t.Fatal("sqlite guarda a chave")
	}
	kept := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin": fake.URL,
		"template": map[string]string{
			"name":    "Alerta de uso",
			"subject": "Uso de {{provider}} em {{used}}%",
			"html":    "<p>{{used}}</p>",
			"text":    "{{used}}",
		},
	})
	if kept.Code != http.StatusOK || strings.Contains(kept.Body, key) || !strings.Contains(kept.Body, `"deploy_key_present":true`) {
		t.Fatalf("manter chave %d", kept.Code)
	}
	again, err := box.Get(context.Background(), sabia.DeploySecret)
	if err != nil || string(again) != key {
		t.Fatal("chave substituída")
	}
	plan := callJSON(t, handler, http.MethodPost, "/v1/admin/sabia/plan", operator, nil)
	if plan.Code != http.StatusOK || strings.Contains(plan.Body, key) || !strings.Contains(plan.Body, `"operation":"criar"`) {
		t.Fatalf("plano %d vazou=%v", plan.Code, strings.Contains(plan.Body, key))
	}
	apply := callJSON(t, handler, http.MethodPost, "/v1/admin/sabia/apply", operator, map[string]any{
		"confirmations": []string{"usage-alert-v1"},
	})
	if apply.Code != http.StatusOK || strings.Contains(apply.Body, key) || !strings.Contains(apply.Body, `"applied":1`) {
		t.Fatalf("aplicar %d vazou=%v", apply.Code, strings.Contains(apply.Body, key))
	}
	joined := strings.Join(hits, "\n")
	if !strings.Contains(joined, `"confirmations":["usage-alert-v1"]`) || strings.Contains(joined, `"deleted"`) {
		t.Fatal("apply não levou a confirmação salva")
	}
	device := enrollDevice(t, handler, operator)
	foreign := callJSON(t, handler, http.MethodGet, "/v1/admin/sabia", device, nil)
	if foreign.Code != http.StatusForbidden || strings.Contains(foreign.Body, key) {
		t.Fatalf("dispositivo %d", foreign.Code)
	}
	bad := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin": "https://user:segredo@sabia.exemplo",
	})
	if bad.Code != http.StatusUnprocessableEntity || strings.Contains(bad.Body, "segredo") {
		t.Fatalf("origem %d", bad.Code)
	}
}

func TestSabiaUpstreamDoesNotEchoTheKey(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := vault.OpenOwn(filepath.Join(dir, "vault"), bytesRepeat())
	if err != nil {
		t.Fatal(err)
	}
	key := "sb_deploy_sintetico"
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"recusada ` + key + `"}`))
	}))
	t.Cleanup(fake.Close)
	handler := (&Server{
		Store:          db,
		Vault:          box,
		AllowOpenSetup: true,
		SabiaHTTP:      fake.Client(),
		Now:            func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	operator := setupOperator(t, handler)
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin":     fake.URL,
		"deploy_key": key,
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, key) {
		t.Fatalf("guardar %d vazou=%v", saved.Code, strings.Contains(saved.Body, key))
	}
	plan := callJSON(t, handler, http.MethodPost, "/v1/admin/sabia/plan", operator, nil)
	if plan.Code != http.StatusBadGateway || !strings.Contains(plan.Body, "UPSTREAM_REJECTED") || strings.Contains(plan.Body, key) {
		t.Fatalf("recusa %d vazou=%v", plan.Code, strings.Contains(plan.Body, key))
	}
}

func TestSabiaKeyWriteNeedsTheVault(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	handler := (&Server{
		Store:          db,
		AllowOpenSetup: true,
		Now:            func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	operator := setupOperator(t, handler)
	key := "sb_deploy_sintetico"
	closed := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin":     "https://sabia.exemplo",
		"deploy_key": key,
	})
	if closed.Code != http.StatusPreconditionFailed || !strings.Contains(closed.Body, "VAULT_UNAVAILABLE") || strings.Contains(closed.Body, key) {
		t.Fatalf("cofre %d vazou=%v", closed.Code, strings.Contains(closed.Body, key))
	}
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"template": map[string]string{
			"name":    "Alerta de uso",
			"subject": "Uso",
			"html":    "<p>oi</p>",
			"text":    "oi",
		},
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, key) || !strings.Contains(saved.Body, "Alerta de uso") {
		t.Fatalf("template %d", saved.Code)
	}
}

func bytesRepeat() []byte {
	return []byte{
		4, 4, 4, 4, 4, 4, 4, 4,
		4, 4, 4, 4, 4, 4, 4, 4,
		4, 4, 4, 4, 4, 4, 4, 4,
		4, 4, 4, 4, 4, 4, 4, 4,
	}
}
