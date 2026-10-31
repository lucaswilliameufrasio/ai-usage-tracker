package httpapi

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
	"testing"
	"time"

	"ai-usage-tracker/internal/access"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

func TestSecretBackendSwitchesOnlyWhenEmpty(t *testing.T) {
	secret := "sintetico-ciphera"
	stored := map[string]string{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer st_sintetico" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			items := make([]map[string]string, 0, len(stored))
			for key, value := range stored {
				items = append(items, map[string]string{"key": key, "value": value})
			}
			_ = json.NewEncoder(w).Encode(items)
		case http.MethodPost:
			var body struct {
				Secrets []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"secrets"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Secrets) != 1 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			stored[body.Secrets[0].Key] = body.Secrets[0].Value
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	own, err := vault.OpenOwn(filepath.Join(dir, "vault"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ciphera := &vault.Ciphera{
		Origin:      fake.URL,
		Project:     "proj",
		Environment: "dev",
		Token:       "st_sintetico",
		HTTP:        fake.Client(),
	}
	handler := (&Server{
		Store:          db,
		AllowOpenSetup: true,
		Now:            func() time.Time { return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC) },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Vault:          own,
		Vaults: map[string]vault.Box{
			access.BackendOwn:     own,
			access.BackendCiphera: ciphera,
		},
	}).Handler()
	operator := setupOperator(t, handler)
	if err := own.Put(context.Background(), "login.github", []byte(secret)); err != nil {
		t.Fatal(err)
	}
	blocked := callJSON(t, handler, http.MethodPut, "/v1/admin/secret-backend", operator, map[string]any{"kind": "ciphera"})
	if blocked.Code != http.StatusConflict || !strings.Contains(blocked.Body, "VAULT_SWITCH_BLOCKED") || strings.Contains(blocked.Body, secret) {
		t.Fatalf("ocupado %d", blocked.Code)
	}
	if err := own.Delete(context.Background(), "login.github"); err != nil {
		t.Fatal(err)
	}
	switched := callJSON(t, handler, http.MethodPut, "/v1/admin/secret-backend", operator, map[string]any{"kind": "ciphera"})
	if switched.Code != http.StatusOK || !strings.Contains(switched.Body, `"kind":"ciphera"`) || !strings.Contains(switched.Body, `"ready":true`) || strings.Contains(switched.Body, secret) {
		t.Fatalf("troca %d %s", switched.Code, switched.Body)
	}
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/login-providers/github", operator, map[string]any{
		"enabled":   true,
		"client_id": "id-pub",
		"secret":    secret,
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, secret) {
		t.Fatalf("gravação %d", saved.Code)
	}
	if stored["login.github"] != secret {
		t.Fatal("o ciphera não recebeu o segredo")
	}
	rawDB, err := os.ReadFile(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rawDB, []byte(secret)) {
		t.Fatal("sqlite guarda o segredo")
	}
}
