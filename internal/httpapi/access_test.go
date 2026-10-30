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

	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

func TestAdminLoginHidesSecret(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracker.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := vault.OpenOwn(filepath.Join(dir, "vault"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{
		Store:          db,
		Vault:          box,
		AllowOpenSetup: true,
		Now:            func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	operator := setupOperator(t, handler)
	secret := "sintetico-segredo-github"
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/login-providers/github", operator, map[string]any{
		"enabled":   true,
		"client_id": "id-pub",
		"secret":    secret,
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, secret) || strings.Contains(saved.Body, `"secret"`) {
		t.Fatalf("gravação %d", saved.Code)
	}
	listed := callJSON(t, handler, http.MethodGet, "/v1/admin/login-providers", operator, nil)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body, secret) {
		t.Fatalf("lista %d", listed.Code)
	}
	got, err := box.Get(context.Background(), "login.github")
	if err != nil || string(got) != secret {
		t.Fatal("cofre")
	}
	rawDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rawDB, []byte(secret)) {
		t.Fatal("sqlite guarda o segredo")
	}
	denied := callJSON(t, handler, http.MethodPut, "/v1/admin/login-providers/github", operator, map[string]any{
		"enabled":   false,
		"client_id": "id-pub",
	})
	if denied.Code != http.StatusConflict || !strings.Contains(denied.Body, "LAST_LOGIN_METHOD") || strings.Contains(denied.Body, secret) {
		t.Fatalf("último %d", denied.Code)
	}
	device := enrollDevice(t, handler, operator)
	foreign := callJSON(t, handler, http.MethodGet, "/v1/admin/login-providers", device, nil)
	if foreign.Code != http.StatusForbidden || strings.Contains(foreign.Body, secret) {
		t.Fatalf("dispositivo %d", foreign.Code)
	}
	other := callJSON(t, handler, http.MethodPut, "/v1/admin/secret-backend", operator, map[string]any{"kind": "ciphera"})
	if other.Code != http.StatusUnprocessableEntity || strings.Contains(other.Body, secret) {
		t.Fatalf("backend %d", other.Code)
	}
	backend := callJSON(t, handler, http.MethodGet, "/v1/admin/secret-backend", operator, nil)
	if backend.Code != http.StatusOK || !strings.Contains(backend.Body, `"kind":"own"`) || !strings.Contains(backend.Body, `"ready":true`) {
		t.Fatalf("cofre ativo %d", backend.Code)
	}
}

func TestUsageSurvivesClosedVault(t *testing.T) {
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
	overview := callJSON(t, handler, http.MethodGet, "/v1/overview", operator, nil)
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body, `"accounts":[]`) {
		t.Fatalf("visão %d", overview.Code)
	}
	backend := callJSON(t, handler, http.MethodGet, "/v1/admin/secret-backend", operator, nil)
	if backend.Code != http.StatusOK || !strings.Contains(backend.Body, `"ready":false`) {
		t.Fatalf("backend %d %s", backend.Code, backend.Body)
	}
	secret := "sintetico-segredo-github"
	closed := callJSON(t, handler, http.MethodPut, "/v1/admin/login-providers/github", operator, map[string]any{
		"enabled":   true,
		"client_id": "id-pub",
		"secret":    secret,
	})
	if closed.Code != http.StatusPreconditionFailed || !strings.Contains(closed.Body, "VAULT_UNAVAILABLE") || strings.Contains(closed.Body, secret) {
		t.Fatalf("fechado %d", closed.Code)
	}
}

type httpResult struct {
	Code int
	Body string
}

func setupOperator(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := post(t, handler, "/v1/setup", "")
	body := readAll(t, response)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("setup %d", response.StatusCode)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["operator_token"] == "" {
		t.Fatal("setup sem token")
	}
	return payload["operator_token"]
}

func enrollDevice(t *testing.T, handler http.Handler, operator string) string {
	t.Helper()
	created := callJSON(t, handler, http.MethodPost, "/v1/enrollment-codes", operator, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("código %d", created.Code)
	}
	var codeBody map[string]any
	if err := json.Unmarshal([]byte(created.Body), &codeBody); err != nil {
		t.Fatal(err)
	}
	code, _ := codeBody["code"].(string)
	enrolled := callJSON(t, handler, http.MethodPost, "/v1/devices", "", map[string]any{
		"code": code,
		"name": "notebook",
	})
	if enrolled.Code != http.StatusCreated {
		t.Fatalf("enrolar %d", enrolled.Code)
	}
	var deviceBody map[string]string
	if err := json.Unmarshal([]byte(enrolled.Body), &deviceBody); err != nil {
		t.Fatal(err)
	}
	if deviceBody["token"] == "" {
		t.Fatal("dispositivo sem token")
	}
	return deviceBody["token"]
}

func callJSON(t *testing.T, handler http.Handler, method string, path string, bearer string, payload any) httpResult {
	t.Helper()
	var reader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	result := recorder.Result()
	return httpResult{Code: result.StatusCode, Body: readAll(t, result)}
}
