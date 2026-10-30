package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ai-usage-tracker/internal/login"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

func TestGitHubSessionIsolatesTheAccount(t *testing.T) {
	const secret = "sintetico-segredo-github"
	const rotated = "sintetico-segredo-rodado"
	const code = "codigo-sintetico"
	var mu sync.Mutex
	seenSecret := ""
	seenVerifier := ""
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/oauth/access_token" {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
			if err != nil {
				t.Errorf("corpo")
				return
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Errorf("form")
				return
			}
			mu.Lock()
			seenSecret = form.Get("client_secret")
			seenVerifier = form.Get("code_verifier")
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"token-falso","token_type":"bearer"}`))
			return
		}
		if r.URL.Path == "/user" {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.Header.Get("Accept") != "application/vnd.github+json" {
				t.Errorf("usuário")
			}
			if strings.Contains(r.Header.Get("Authorization"), secret) || strings.Contains(r.Header.Get("Authorization"), rotated) {
				t.Errorf("usuário recebeu o segredo")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"email":null}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(idp.Close)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracker.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := vault.OpenOwn(filepath.Join(dir, "vault"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }
	flow := &login.Flow{
		Store: db,
		Vault: box,
		HTTP: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		Ends: login.Endpoints{
			GitHubAuthorize: idp.URL + "/login/oauth/authorize",
			GitHubToken:     idp.URL + "/login/oauth/access_token",
			GitHubUser:      idp.URL + "/user",
		},
		Now: now,
	}
	handler := (&Server{
		Store:          db,
		Vault:          box,
		Login:          flow,
		AllowOpenSetup: true,
		Now:            now,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	operator := setupOperator(t, handler)
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/login-providers/github", operator, map[string]any{
		"enabled":   true,
		"client_id": "id-pub",
		"secret":    secret,
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, secret) {
		t.Fatalf("gravação %d", saved.Code)
	}
	listed := callJSON(t, handler, http.MethodGet, "/v1/login/providers", "", nil)
	if listed.Code != http.StatusOK || listed.Body != "{\"data\":[{\"provider\":\"github\"}]}\n" || strings.Contains(listed.Body, secret) || strings.Contains(listed.Body, "id-pub") {
		t.Fatalf("públicos %d", listed.Code)
	}

	started := trip(t, handler, http.MethodGet, "/v1/login/github/start", "", nil, nil)
	if started.Code != http.StatusFound || strings.Contains(started.Location, secret) || !strings.Contains(started.Location, "code_challenge_method=S256") {
		t.Fatalf("início %d", started.Code)
	}
	state := queryOf(t, started.Location, "state")
	turned := callJSON(t, handler, http.MethodPut, "/v1/admin/login-providers/github", operator, map[string]any{
		"enabled":   true,
		"client_id": "id-pub",
		"secret":    rotated,
	})
	if turned.Code != http.StatusOK || strings.Contains(turned.Body, secret) || strings.Contains(turned.Body, rotated) {
		t.Fatalf("rotação %d", turned.Code)
	}
	callback := trip(t, handler, http.MethodGet, "/v1/login/github/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), "", nil, nil)
	if callback.Code != http.StatusFound || strings.Contains(callback.Location, code) || strings.Contains(callback.Location, secret) || strings.Contains(callback.Location, rotated) {
		t.Fatalf("retorno %d", callback.Code)
	}
	success, err := url.Parse(callback.Location)
	if err != nil || success.Path != "/" || success.RawQuery != "" {
		t.Fatal("destino")
	}
	session := cookieNamed(callback.Cookies, "tracker_session")
	if session == nil || !session.HttpOnly || session.Secure || session.SameSite != http.SameSiteLaxMode || session.Path != "/" || !strings.HasPrefix(session.Value, "ses_") || session.MaxAge != 14*24*3600 {
		t.Fatal("cookie")
	}
	mu.Lock()
	gotSecret := seenSecret
	gotVerifier := seenVerifier
	mu.Unlock()
	if gotSecret != secret || gotVerifier == "" {
		t.Fatal("segredo da tentativa")
	}
	stored, err := box.Get(context.Background(), "login.github")
	if err != nil || string(stored) != rotated {
		t.Fatal("cofre")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "vault"))
	if err != nil || len(entries) != 1 {
		t.Fatal("cópia da tentativa")
	}
	rawDB, err := os.ReadFile(dbPath)
	if err != nil || bytes.Contains(rawDB, []byte(secret)) || bytes.Contains(rawDB, []byte(rotated)) {
		t.Fatal("sqlite guarda o segredo")
	}

	replay := trip(t, handler, http.MethodGet, "/v1/login/github/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), "", nil, nil)
	if replay.Code != http.StatusFound || !strings.Contains(replay.Location, "login=falha") || strings.Contains(replay.Location, code) {
		t.Fatalf("reuso %d", replay.Code)
	}
	current := trip(t, handler, http.MethodGet, "/v1/session", "", session, nil)
	if current.Code != http.StatusOK || !strings.Contains(current.Body, `"provider":"github"`) || strings.Contains(current.Body, session.Value) || strings.Contains(current.Body, secret) {
		t.Fatalf("sessão %d", current.Code)
	}
	admin := trip(t, handler, http.MethodGet, "/v1/admin/login-providers", "", session, nil)
	if admin.Code != http.StatusUnauthorized || strings.Contains(admin.Body, secret) {
		t.Fatalf("admin %d", admin.Code)
	}

	personDevice, personToken := enrollNamed(t, handler, "", session, "caderno")
	officeDevice, officeToken := enrollNamed(t, handler, operator, nil, "mesa")
	postSample(t, handler, personToken, "pessoa-aaaa")
	postSample(t, handler, officeToken, "outra-bbbb")
	personSamples := trip(t, handler, http.MethodGet, "/v1/account/samples?limit=20", "", session, nil)
	if personSamples.Code != http.StatusOK || !strings.Contains(personSamples.Body, "pessoa-aaaa") || strings.Contains(personSamples.Body, "outra-bbbb") {
		t.Fatalf("amostras da pessoa %d", personSamples.Code)
	}
	allSamples := callJSON(t, handler, http.MethodGet, "/v1/samples?limit=20", operator, nil)
	if allSamples.Code != http.StatusOK || !strings.Contains(allSamples.Body, "pessoa-aaaa") || !strings.Contains(allSamples.Body, "outra-bbbb") {
		t.Fatalf("amostras da instalação %d", allSamples.Code)
	}
	personOverview := trip(t, handler, http.MethodGet, "/v1/account/overview", "", session, nil)
	if personOverview.Code != http.StatusOK || strings.Contains(personOverview.Body, "outra-bbbb") || !strings.Contains(personOverview.Body, "pessoa-aaaa") {
		t.Fatalf("visão da pessoa %d", personOverview.Code)
	}
	personDevices := trip(t, handler, http.MethodGet, "/v1/account/devices", "", session, nil)
	if personDevices.Code != http.StatusOK || !strings.Contains(personDevices.Body, "caderno") || strings.Contains(personDevices.Body, "mesa") {
		t.Fatalf("dispositivos da pessoa %d", personDevices.Code)
	}
	denied := trip(t, handler, http.MethodPost, "/v1/account/devices/"+url.PathEscape(officeDevice)+"/revoke", "", session, map[string]any{})
	if denied.Code != http.StatusNotFound {
		t.Fatalf("revogação alheia %d", denied.Code)
	}
	revoked := trip(t, handler, http.MethodPost, "/v1/account/devices/"+url.PathEscape(personDevice)+"/revoke", "", session, map[string]any{})
	if revoked.Code != http.StatusOK || !strings.Contains(revoked.Body, "caderno") {
		t.Fatalf("revogação própria %d", revoked.Code)
	}
	cookieOnly := trip(t, handler, http.MethodPost, "/v1/samples", "", session, map[string]any{
		"sample_id":    "recusada-cccc",
		"provider":     "claude",
		"collected_at": "2026-10-30T12:00:00Z",
		"windows":      []any{},
	})
	if cookieOnly.Code != http.StatusUnauthorized || strings.Contains(cookieOnly.Body, secret) {
		t.Fatalf("ingestão %d", cookieOnly.Code)
	}
	deviceSession := trip(t, handler, http.MethodGet, "/v1/session", personToken, nil, nil)
	if deviceSession.Code != http.StatusUnauthorized {
		t.Fatalf("dispositivo %d", deviceSession.Code)
	}
	loggedOut := trip(t, handler, http.MethodPost, "/v1/session/logout", "", session, nil)
	if loggedOut.Code != http.StatusNoContent {
		t.Fatalf("saída %d", loggedOut.Code)
	}
	after := trip(t, handler, http.MethodGet, "/v1/session", "", session, nil)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("sessão encerrada %d", after.Code)
	}
}

func TestPublicLoginWhenVaultClosed(t *testing.T) {
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
	listed := callJSON(t, handler, http.MethodGet, "/v1/login/providers", "", nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body, `"data":[]`) {
		t.Fatalf("lista %d", listed.Code)
	}
	started := trip(t, handler, http.MethodGet, "/v1/login/github/start", "", nil, nil)
	if started.Code != http.StatusFound || !strings.Contains(started.Location, "login=falha") {
		t.Fatalf("início %d", started.Code)
	}
	overview := callJSON(t, handler, http.MethodGet, "/v1/overview", operator, nil)
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body, "claude") {
		t.Fatalf("visão %d", overview.Code)
	}
}

func enrollNamed(t *testing.T, handler http.Handler, bearer string, cookie *http.Cookie, name string) (string, string) {
	t.Helper()
	path := "/v1/account/enrollment-codes"
	if bearer != "" {
		path = "/v1/enrollment-codes"
	}
	created := trip(t, handler, http.MethodPost, path, bearer, cookie, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("código %d", created.Code)
	}
	var codeBody map[string]any
	if err := json.Unmarshal([]byte(created.Body), &codeBody); err != nil {
		t.Fatal(err)
	}
	code, _ := codeBody["code"].(string)
	enrolled := trip(t, handler, http.MethodPost, "/v1/devices", "", nil, map[string]any{"code": code, "name": name})
	if enrolled.Code != http.StatusCreated {
		t.Fatalf("enrolar %d", enrolled.Code)
	}
	var deviceBody map[string]string
	if err := json.Unmarshal([]byte(enrolled.Body), &deviceBody); err != nil {
		t.Fatal(err)
	}
	if deviceBody["device_id"] == "" || deviceBody["token"] == "" {
		t.Fatal("dispositivo incompleto")
	}
	return deviceBody["device_id"], deviceBody["token"]
}

func postSample(t *testing.T, handler http.Handler, deviceToken string, sampleID string) {
	t.Helper()
	posted := trip(t, handler, http.MethodPost, "/v1/samples", deviceToken, nil, map[string]any{
		"sample_id":    sampleID,
		"provider":     "claude",
		"collected_at": "2026-10-30T12:00:00Z",
		"windows": []any{
			map[string]any{
				"name":      "five_hour",
				"used":      10,
				"limit":     100,
				"unit":      "percent",
				"resets_at": "2026-10-30T17:00:00Z",
			},
		},
	})
	if posted.Code != http.StatusCreated {
		t.Fatalf("amostra %d", posted.Code)
	}
}

type tripResult struct {
	Code     int
	Body     string
	Location string
	Cookies  []*http.Cookie
}

func trip(t *testing.T, handler http.Handler, method string, path string, bearer string, cookie *http.Cookie, payload any) tripResult {
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
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	result := recorder.Result()
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	return tripResult{Code: result.StatusCode, Body: string(body), Location: result.Header.Get("Location"), Cookies: result.Cookies()}
}

func cookieNamed(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func queryOf(t *testing.T, location string, name string) string {
	t.Helper()
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	value := parsed.Query().Get(name)
	if value == "" {
		t.Fatal("parâmetro ausente")
	}
	return value
}
