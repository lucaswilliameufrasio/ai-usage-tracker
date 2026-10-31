package httpapi

import (
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

	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

type sabiaHit struct {
	Path        string
	Auth        string
	Idempotency string
	Body        string
}

type sabiaCapture struct {
	mu   sync.Mutex
	hits []sabiaHit
	code int
	body string
}

func (c *sabiaCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.hits = append(c.hits, sabiaHit{
		Path:        r.URL.Path,
		Auth:        r.Header.Get("Authorization"),
		Idempotency: r.Header.Get("Idempotency-Key"),
		Body:        string(raw),
	})
	code := c.code
	body := c.body
	c.mu.Unlock()
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}

func (c *sabiaCapture) snapshot() []sabiaHit {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]sabiaHit, len(c.hits))
	copy(out, c.hits)
	return out
}

func openSabiaSend(t *testing.T, code int, body string) (*store.Store, *Server, http.Handler, *sabiaCapture, string, string) {
	t.Helper()
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
	capture := &sabiaCapture{code: code, body: body}
	fake := httptest.NewServer(capture)
	t.Cleanup(fake.Close)
	api := &Server{
		Store:          db,
		Vault:          box,
		AllowOpenSetup: true,
		SabiaHTTP:      fake.Client(),
		Now:            func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return db, api, api.Handler(), capture, dbPath, fake.URL
}

func TestSabiaSendAcceptsIntoTheGatewayQueue(t *testing.T) {
	const sendKey = "sb_send_sintetico"
	const deployKey = "sb_deploy_sintetico"
	db, api, handler, capture, dbPath, origin := openSabiaSend(t, http.StatusAccepted, `{"delivery_id":"4f3b0a0e-6a1c-4d2e-9b7a-1c2d3e4f5a6b","status":"sent","template":"usage.alert","template_version":1}`)
	operator := setupOperator(t, handler)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	if len(capture.snapshot()) != 0 {
		t.Fatal("enviou sem destinatário nem chave")
	}
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin":     origin,
		"recipient":  "pessoa@exemplo.com",
		"deploy_key": deployKey,
		"send_key":   sendKey,
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, sendKey) || strings.Contains(saved.Body, deployKey) {
		t.Fatalf("%d %s", saved.Code, saved.Body)
	}
	if !strings.Contains(saved.Body, `"send_key_present":true`) || !strings.Contains(saved.Body, `"recipient":"pessoa@exemplo.com"`) {
		t.Fatal(saved.Body)
	}
	rawDB, err := os.ReadFile(dbPath)
	if err != nil || strings.Contains(string(rawDB), sendKey) || strings.Contains(string(rawDB), deployKey) {
		t.Fatal("chave no sqlite")
	}
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	hits := capture.snapshot()
	if len(hits) != 1 || hits[0].Path != "/v1/messages" || hits[0].Auth != "Bearer "+sendKey {
		t.Fatalf("%+v", hits)
	}
	if hits[0].Auth == "Bearer "+deployKey || hits[0].Idempotency == "" || strings.Contains(hits[0].Idempotency, sendKey) {
		t.Fatalf("%+v", hits[0])
	}
	if strings.Contains(hits[0].Body, `"from"`) || strings.Contains(hits[0].Body, sendKey) || strings.Contains(hits[0].Body, "usage-alert-v1") {
		t.Fatal(hits[0].Body)
	}
	for _, part := range []string{`"channel":"email"`, `"template":"usage.alert"`, `"locale":"pt-BR"`, `"to":"pessoa@exemplo.com"`, `"provider":"Claude"`, `"window":"five_hour"`, `"used":"90"`, `"resets_at":"30/10/2026 17:00"`} {
		if !strings.Contains(hits[0].Body, part) {
			t.Fatalf("falta %s em %s", part, hits[0].Body)
		}
	}
	rows, err := db.ListSabiaDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "queued" || rows[0].Detail != "QUEUED:4f3b0a0e-6a1c-4d2e-9b7a-1c2d3e4f5a6b" {
		t.Fatalf("%+v %v", rows, err)
	}
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	if len(capture.snapshot()) != 1 {
		t.Fatal("pediu o envio de novo")
	}
}

func TestSabiaSendRetriesThenStaysQueued(t *testing.T) {
	db, api, handler, capture, _, origin := openSabiaSend(t, http.StatusServiceUnavailable, `{"message":"fila cheia sb_send_sintetico"}`)
	operator := setupOperator(t, handler)
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin":    origin,
		"recipient": "pessoa@exemplo.com",
		"send_key":  "sb_send_sintetico",
	})
	if saved.Code != http.StatusOK || strings.Contains(saved.Body, "sb_send_sintetico") {
		t.Fatalf("%d %s", saved.Code, saved.Body)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListSabiaDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" || rows[0].Attempts != 1 || strings.Contains(rows[0].Detail, "sb_send_sintetico") {
		t.Fatalf("%+v %v", rows, err)
	}
	capture.mu.Lock()
	capture.code = http.StatusAccepted
	capture.body = `{"delivery_id":"dlv-sintetico","status":"queued"}`
	capture.mu.Unlock()
	api.Now = func() time.Time { return rows[0].NextAttempt }
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	rows, err = db.ListSabiaDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "queued" || rows[0].Detail != "QUEUED:dlv-sintetico" {
		t.Fatalf("%+v %v", rows, err)
	}
	hits := capture.snapshot()
	if len(hits) != 2 || hits[0].Idempotency == "" || hits[0].Idempotency != hits[1].Idempotency {
		t.Fatalf("%+v", hits)
	}
}

func TestSabiaSendRefusesTheKeyWithoutRetry(t *testing.T) {
	db, api, handler, capture, _, origin := openSabiaSend(t, http.StatusUnauthorized, `{"message":"recusada sb_send_sintetico"}`)
	operator := setupOperator(t, handler)
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin":    origin,
		"recipient": "pessoa@exemplo.com",
		"send_key":  "sb_send_sintetico",
	})
	if saved.Code != http.StatusOK {
		t.Fatal(saved.Body)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListSabiaDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Detail != "SEND_FAILED" || rows[0].Attempts != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
	if strings.Contains(rows[0].Detail, "sb_send_sintetico") {
		t.Fatal(rows[0].Detail)
	}
	api.Now = func() time.Time { return now.Add(time.Hour) }
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(capture.snapshot()) != 1 {
		t.Fatal("repetiu uma recusa final")
	}
}

func TestSabiaReplacingTheSendKeyDropsThePendingAlert(t *testing.T) {
	_, api, handler, capture, _, origin := openSabiaSend(t, http.StatusServiceUnavailable, `{}`)
	operator := setupOperator(t, handler)
	saved := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"origin":    origin,
		"recipient": "pessoa@exemplo.com",
		"send_key":  "sb_send_sintetico",
	})
	if saved.Code != http.StatusOK {
		t.Fatal(saved.Body)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	replaced := callJSON(t, handler, http.MethodPut, "/v1/admin/sabia", operator, map[string]any{
		"send_key": "sb_send_nova_sintetica",
	})
	if replaced.Code != http.StatusOK || strings.Contains(replaced.Body, "sb_send_nova_sintetica") {
		t.Fatalf("%d %s", replaced.Code, replaced.Body)
	}
	rows, err := api.Store.ListSabiaDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Detail != "SABIA_REPLACED" {
		t.Fatalf("%+v %v", rows, err)
	}
	api.Now = func() time.Time { return now.Add(time.Hour) }
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	hits := capture.snapshot()
	if len(hits) != 1 || strings.Contains(hits[0].Auth, "sb_send_nova_sintetica") {
		t.Fatalf("%+v", hits)
	}
}
