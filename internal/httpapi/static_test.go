package httpapi

import (
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
)

func TestStaticShellDoesNotEscapeWebDir(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "web")
	if err := os.Mkdir(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("SHELL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "mark.txt"), []byte("MARK"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{WebDir: web, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	home := getBody(t, handler, "/")
	if !strings.Contains(home, "SHELL") || strings.Contains(home, "SECRET") {
		t.Fatalf("home => %s", home)
	}
	mark := getBody(t, handler, "/mark.txt")
	if mark != "MARK" {
		t.Fatalf("asset => %s", mark)
	}
	for _, path := range []string{"/secret.txt", "/../secret.txt", "/%2e%2e/secret.txt"} {
		response := get(t, handler, path)
		body := readAll(t, response)
		if response.StatusCode == http.StatusOK && strings.Contains(body, "SECRET") {
			t.Fatalf("%s vazou o arquivo externo", path)
		}
	}
	for _, path := range []string{"/acesso", "/historico", "/dispositivos", "/alertas", "/access"} {
		response := get(t, handler, path)
		body := readAll(t, response)
		if response.StatusCode != http.StatusOK || body != "SHELL" {
			t.Fatalf("%s => %d %s", path, response.StatusCode, body)
		}
	}
	missing := get(t, handler, "/assets/ausente.js")
	if missing.StatusCode == http.StatusOK || strings.Contains(readAll(t, missing), "SHELL") {
		t.Fatalf("asset ausente devolveu o shell")
	}
	api := get(t, handler, "/v1/inexistente")
	if api.StatusCode == http.StatusOK || strings.Contains(readAll(t, api), "SHELL") {
		t.Fatalf("rota de API devolveu o shell")
	}
}

func TestSetupTokenIsRequiredWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	handler := (&Server{
		Store:          db,
		Now:            func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) },
		AllowOpenSetup: true,
		SetupToken:     "setup-secret",
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	denied := post(t, handler, "/v1/setup", "")
	if denied.StatusCode != http.StatusForbidden || !strings.Contains(readAll(t, denied), "SETUP_TOKEN_REQUIRED") {
		t.Fatalf("status %d", denied.StatusCode)
	}
	allowed := post(t, handler, "/v1/setup", "setup-secret")
	if allowed.StatusCode != http.StatusCreated {
		t.Fatalf("com token: %d %s", allowed.StatusCode, readAll(t, allowed))
	}
}

func TestSetupStaysClosedOffLoopbackWithoutToken(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	handler := (&Server{
		Store:          db,
		AllowOpenSetup: false,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	response := post(t, handler, "/v1/setup", "")
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", response.StatusCode)
	}
}

func get(t *testing.T, handler http.Handler, path string) *http.Response {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Result()
}

func getBody(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	return readAll(t, get(t, handler, path))
}

func post(t *testing.T, handler http.Handler, path string, setupToken string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, nil)
	if setupToken != "" {
		request.Header.Set("X-Setup-Token", setupToken)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Result()
}

func readAll(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
