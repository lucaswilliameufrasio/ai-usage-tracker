package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ai-usage-tracker/internal/mail"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

func TestEmailHidesTheKeyAndSendsThroughTheVault(t *testing.T) {
	const key = "re_sintetico_nao_usar"
	db, api, handler, capture, dbPath := openEmail(t, http.StatusOK, `{"id":"msg_sintetico"}`)
	operator := setupOperator(t, handler)
	denied := callJSON(t, handler, http.MethodGet, "/v1/admin/email", "", nil)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("acesso %d", denied.Code)
	}
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/email", operator, map[string]any{
		"provider":     "resend",
		"from_address": "Avisos <avisos@exemplo.com>",
		"to_address":   "pessoa@exemplo.com",
		"active":       true,
		"api_key":      key,
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, key) || strings.Contains(saved.Body, "api_key") {
		t.Fatalf("%d %s", saved.Code, saved.Body)
	}
	if !strings.Contains(saved.Body, `"secret_present":true`) || !strings.Contains(saved.Body, `"provider":"resend"`) {
		t.Fatal(saved.Body)
	}
	got := callJSON(t, handler, http.MethodGet, "/v1/admin/email", operator, nil)
	if got.Code != http.StatusOK || strings.Contains(got.Body, key) {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	rawDB, err := os.ReadFile(dbPath)
	if err != nil || bytes.Contains(rawDB, []byte(key)) {
		t.Fatal("chave no sqlite")
	}
	missing := callJSON(t, handler, http.MethodPut, "/v1/admin/email", operator, map[string]any{
		"provider":     "resend",
		"from_address": "Avisos <avisos@exemplo.com>",
		"to_address":   "pessoa@exemplo.com",
		"active":       true,
	})
	if missing.Code != http.StatusOK || strings.Contains(missing.Body, key) {
		t.Fatalf("manter %d %s", missing.Code, missing.Body)
	}
	if err := api.planAlerts(alertSample(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != 1 || capture.requests[0].Path != "/emails" || capture.requests[0].Auth != "Bearer "+key {
		t.Fatalf("%+v", capture.requests)
	}
	rows, err := db.ListEmailDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "sent" {
		t.Fatalf("%+v %v", rows, err)
	}
	if err := api.planAlerts(alertSample(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != 1 {
		t.Fatal("duplicou o e-mail")
	}
}

func TestEmailActivateRequiresAKey(t *testing.T) {
	_, _, handler, _, _ := openEmail(t, http.StatusOK, `{"id":"msg_sintetico"}`)
	operator := setupOperator(t, handler)
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/email", operator, map[string]any{
		"provider":     "resend",
		"from_address": "avisos@exemplo.com",
		"to_address":   "pessoa@exemplo.com",
		"active":       true,
	})
	if saved.Code != http.StatusUnprocessableEntity || !strings.Contains(saved.Body, "Informe a chave.") {
		t.Fatalf("%d %s", saved.Code, saved.Body)
	}
}

func TestEmailProvidersHitTheDocumentedPath(t *testing.T) {
	cases := []struct {
		provider string
		path     string
		body     map[string]any
		auth     string
	}{
		{
			provider: "millionsend",
			path:     "/emails",
			auth:     "Bearer ms_sintetico_nao_usar",
			body: map[string]any{
				"provider": "millionsend", "from_address": "avisos@exemplo.com", "to_address": "pessoa@exemplo.com",
				"active": true, "api_key": "ms_sintetico_nao_usar",
			},
		},
		{
			provider: "cloudflare",
			path:     "/client/v4/accounts/conta123/email/sending/send",
			auth:     "Bearer cf_sintetico_nao_usar",
			body: map[string]any{
				"provider": "cloudflare", "from_address": "avisos@exemplo.com", "to_address": "pessoa@exemplo.com",
				"account_ref": "conta123", "active": true, "api_key": "cf_sintetico_nao_usar",
			},
		},
		{
			provider: "ses",
			path:     "/v2/email/outbound-emails",
			body: map[string]any{
				"provider": "ses", "from_address": "avisos@exemplo.com", "to_address": "pessoa@exemplo.com",
				"account_ref": "sa-east-1", "active": true, "access_key_id": "AKIAEXEMPLO", "secret_access_key": "ses_sintetico_nao_usar",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			reply := `{"id":"msg_sintetico"}`
			if tc.provider == "cloudflare" {
				reply = `{"success":true,"errors":[],"messages":[],"result":{"delivered":["pessoa@exemplo.com"],"permanent_bounces":[],"queued":[]}}`
			}
			if tc.provider == "ses" {
				reply = `{"MessageId":"msg_sintetico"}`
			}
			db, api, handler, capture, _ := openEmail(t, http.StatusOK, reply)
			operator := setupOperator(t, handler)
			saved := callJSON(t, handler, http.MethodPut, "/v1/admin/email", operator, tc.body)
			if saved.Code != http.StatusOK || strings.Contains(saved.Body, "nao_usar") || strings.Contains(saved.Body, "AKIAEXEMPLO") {
				t.Fatalf("%d %s", saved.Code, saved.Body)
			}
			if err := api.planAlerts(alertSample(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))); err != nil {
				t.Fatal(err)
			}
			if len(capture.requests) != 1 || capture.requests[0].Path != tc.path {
				t.Fatalf("%+v", capture.requests)
			}
			if tc.auth != "" && capture.requests[0].Auth != tc.auth {
				t.Fatal(capture.requests[0].Auth)
			}
			if tc.provider == "ses" && !strings.Contains(capture.requests[0].Auth, "Credential=AKIAEXEMPLO/") {
				t.Fatal(capture.requests[0].Auth)
			}
			if tc.provider == "ses" && strings.Contains(capture.requests[0].Auth, "ses_sintetico_nao_usar") {
				t.Fatal("segredo na assinatura")
			}
			rows, err := db.ListEmailDeliveries(5)
			if err != nil || len(rows) != 1 || rows[0].Status != "sent" {
				t.Fatalf("%+v %v", rows, err)
			}
		})
	}
}

func TestEmailRetriesThenSends(t *testing.T) {
	db, api, handler, capture, _ := openEmail(t, http.StatusBadGateway, `{}`)
	operator := setupOperator(t, handler)
	saveEmailKey(t, handler, operator, "re_sintetico_nao_usar")
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListEmailDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" || rows[0].Attempts != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
	capture.code = http.StatusOK
	capture.body = `{"id":"msg_sintetico"}`
	api.Now = func() time.Time { return rows[0].NextAttempt }
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	rows, err = db.ListEmailDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "sent" || len(capture.requests) != 2 {
		t.Fatalf("%+v %d %v", rows, len(capture.requests), err)
	}
}

func TestEmailPermanentFailureDoesNotRetry(t *testing.T) {
	db, api, handler, capture, _ := openEmail(t, http.StatusBadRequest, `{"message":"nao"}`)
	operator := setupOperator(t, handler)
	saveEmailKey(t, handler, operator, "re_sintetico_nao_usar")
	if err := api.planAlerts(alertSample(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListEmailDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Detail != "SEND_FAILED" {
		t.Fatalf("%+v %v", rows, err)
	}
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != 1 {
		t.Fatalf("tentou de novo %d", len(capture.requests))
	}
}

func TestEmailInactiveDoesNotSend(t *testing.T) {
	db, api, handler, capture, _ := openEmail(t, http.StatusOK, `{"id":"msg_sintetico"}`)
	operator := setupOperator(t, handler)
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/email", operator, map[string]any{
		"provider": "resend", "from_address": "avisos@exemplo.com", "to_address": "pessoa@exemplo.com",
		"active": false, "api_key": "re_sintetico_nao_usar",
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, "re_sintetico_nao_usar") {
		t.Fatalf("%d %s", saved.Code, saved.Body)
	}
	if err := api.planAlerts(alertSample(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListEmailDeliveries(5)
	if err != nil || len(rows) != 0 || len(capture.requests) != 0 {
		t.Fatalf("%+v %d %v", rows, len(capture.requests), err)
	}
}

func TestEmailReplaceAndDeleteDropPending(t *testing.T) {
	db, api, handler, capture, _ := openEmail(t, http.StatusBadGateway, `{}`)
	operator := setupOperator(t, handler)
	saveEmailKey(t, handler, operator, "re_sintetico_nao_usar")
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	replaced := callJSON(t, handler, http.MethodPut, "/v1/admin/email", operator, map[string]any{
		"provider": "resend", "from_address": "avisos@exemplo.com", "to_address": "pessoa@exemplo.com",
		"active": true, "api_key": "re_outra_sintetica",
	})
	if replaced.Code != http.StatusOK || strings.Contains(replaced.Body, "re_outra_sintetica") || strings.Contains(replaced.Body, "re_sintetico_nao_usar") {
		t.Fatalf("%d %s", replaced.Code, replaced.Body)
	}
	rows, err := db.ListEmailDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Detail != "EMAIL_REPLACED" {
		t.Fatalf("%+v %v", rows, err)
	}
	capture.code = http.StatusOK
	capture.body = `{"id":"msg_sintetico"}`
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != 1 || capture.requests[0].Auth != "Bearer re_sintetico_nao_usar" {
		t.Fatalf("%+v", capture.requests)
	}
	capture.code = http.StatusBadGateway
	if err := api.planAlerts(alertSample(now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != 2 || capture.requests[1].Auth != "Bearer re_outra_sintetica" {
		t.Fatalf("%+v", capture.requests)
	}
	removed := callJSON(t, handler, http.MethodDelete, "/v1/admin/email", operator, nil)
	if removed.Code != http.StatusOK || strings.Contains(removed.Body, "re_outra") {
		t.Fatalf("%d %s", removed.Code, removed.Body)
	}
	rows, err = db.ListEmailDeliveries(5)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Detail == "EMAIL_REMOVED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", rows)
	}
	before := len(capture.requests)
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != before {
		t.Fatal("apagado ainda enviou")
	}
}

func TestEmailQuietSuppressesWithoutSending(t *testing.T) {
	db, api, handler, capture, _ := openEmail(t, http.StatusOK, `{"id":"msg_sintetico"}`)
	operator := setupOperator(t, handler)
	saveEmailKey(t, handler, operator, "re_sintetico_nao_usar")
	if err := db.SaveSettings(store.Settings{
		StaleAfter: time.Hour, AlertThreshold: 80, QuietStart: "11:00", QuietEnd: "13:00", Timezone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	if err := api.planAlerts(alertSample(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListEmailDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "suppressed" || rows[0].Detail != "QUIET_HOURS" || len(capture.requests) != 0 {
		t.Fatalf("%+v %d %v", rows, len(capture.requests), err)
	}
}

func TestEmailSendsWhenPushIsOff(t *testing.T) {
	_, api, handler, capture, _ := openEmail(t, http.StatusOK, `{"id":"msg_sintetico"}`)
	if api.Sender != nil {
		t.Fatal("push ligado")
	}
	operator := setupOperator(t, handler)
	saveEmailKey(t, handler, operator, "re_sintetico_nao_usar")
	if err := api.planAlerts(alertSample(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != 1 {
		t.Fatalf("envios %d", len(capture.requests))
	}
	rows, err := api.Store.ListDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "unconfigured" {
		t.Fatalf("%+v %v", rows, err)
	}
}

type seenMail struct {
	Path string
	Auth string
}

type mailCapture struct {
	mu       sync.Mutex
	code     int
	body     string
	requests []seenMail
}

func (c *mailCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, seenMail{Path: r.URL.Path, Auth: r.Header.Get("Authorization")})
	w.WriteHeader(c.code)
	_, _ = io.WriteString(w, c.body)
}

func openEmail(t *testing.T, code int, body string) (*store.Store, *Server, http.Handler, *mailCapture, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracker.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := vault.OpenOwn(filepath.Join(dir, "vault"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	capture := &mailCapture{code: code, body: body}
	server := httptest.NewServer(capture)
	t.Cleanup(server.Close)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	api := &Server{
		Store:          db,
		Vault:          box,
		AllowOpenSetup: true,
		Now:            func() time.Time { return now },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mail: &mail.Client{
			HTTP:        server.Client(),
			Resend:      server.URL,
			MillionSend: server.URL,
			Cloudflare:  server.URL,
			SESBase:     server.URL,
			Now:         func() time.Time { return now },
		},
	}
	return db, api, api.Handler(), capture, dbPath
}

func saveEmailKey(t *testing.T, handler http.Handler, operator string, key string) {
	t.Helper()
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/email", operator, map[string]any{
		"provider": "resend", "from_address": "avisos@exemplo.com", "to_address": "pessoa@exemplo.com",
		"active": true, "api_key": key,
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, key) {
		t.Fatalf("%d %s", saved.Code, saved.Body)
	}
}
