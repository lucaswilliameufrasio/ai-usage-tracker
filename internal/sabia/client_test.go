package sabia

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlanTranslatesAndHidesRejectedKey(t *testing.T) {
	key := "sb_deploy_sintetico"
	var planBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/template-deployments/plan" {
			t.Fatalf("caminho %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		planBody = string(raw)
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"recusada ` + key + `"}`))
			return
		}
		_, _ = w.Write([]byte(`{"changes":[{"id":"usage-alert-v1","operation":"create","confirmation_required":true}]}`))
	}))
	t.Cleanup(server.Close)
	manifest, err := Manifest(Link{}, DefaultTemplate())
	if err != nil {
		t.Fatal(err)
	}
	denied, err := PlanDeployment(context.Background(), server.Client(), server.URL, "outra-chave", manifest)
	if err == nil || denied.Changes != nil {
		t.Fatal("chave errada passou")
	}
	if strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "outra-chave") {
		t.Fatal("erro devolveu a chave")
	}
	if !strings.Contains(err.Error(), "recusada") {
		t.Fatal(err)
	}
	plan, err := PlanDeployment(context.Background(), server.Client(), server.URL, key, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Operation != "criar" || !plan.Changes[0].ConfirmationRequired {
		t.Fatalf("%+v", plan.Changes)
	}
	if !strings.Contains(planBody, `"application_id":null`) {
		t.Fatal("manifesto")
	}
}

func TestApplySendsConfirmations(t *testing.T) {
	key := "sb_deploy_sintetico"
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Fatal("autorização")
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"applied":1,"unchanged":0,"deleted":0}`))
	}))
	t.Cleanup(server.Close)
	manifest, err := Manifest(Link{}, DefaultTemplate())
	if err != nil {
		t.Fatal(err)
	}
	empty, err := ApplyDeployment(context.Background(), server.Client(), server.URL, key, manifest, nil)
	if err != nil || empty.Applied != 1 {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"confirmations":[]`) {
		t.Fatalf("confirmações %s", body)
	}
	_, err = ApplyDeployment(context.Background(), server.Client(), server.URL, key, manifest, []string{"usage-alert-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"confirmations":["usage-alert-v1"]`) {
		t.Fatalf("confirmação %s", body)
	}
	if strings.Contains(body, `\u003c`) {
		t.Fatal("html escapado")
	}
}

func TestSendMessagePostsTheApplicationKeyAndKeeps202Queued(t *testing.T) {
	sendKey := "sb_send_sintetico"
	status := http.StatusAccepted
	reply := `{"delivery_id":"4f3b0a0e-6a1c-4d2e-9b7a-1c2d3e4f5a6b","status":"sent","template":"usage.alert","template_version":1}`
	var method string
	var path string
	var auth string
	var idempotency string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		idempotency = r.Header.Get("Idempotency-Key")
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		if auth != "Bearer "+sendKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"recusada ` + sendKey + `"}`))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	denied, err := SendMessage(context.Background(), server.Client(), server.URL, "outra-chave", "alerta-1", Message{
		To: "pessoa@exemplo.com", Template: TemplateKey, Locale: "pt-BR",
		Data: map[string]string{"provider": "Claude", "window": "five_hour", "used": "90", "resets_at": "30/10/2026 17:00"},
	})
	if err == nil || denied.DeliveryID != "" {
		t.Fatal("chave errada passou")
	}
	var call *CallError
	if !errors.As(err, &call) || !call.Permanent() {
		t.Fatalf("permanente %v", err)
	}
	if strings.Contains(err.Error(), sendKey) || strings.Contains(err.Error(), "outra-chave") {
		t.Fatal("erro devolveu a chave")
	}
	accepted, err := SendMessage(context.Background(), server.Client(), server.URL, sendKey, "alerta-1", Message{
		To: "pessoa@exemplo.com", Template: TemplateKey, Locale: "pt-BR",
		Data: map[string]string{"provider": "Claude", "window": "five_hour", "used": "90", "resets_at": "30/10/2026 17:00"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/v1/messages" || auth != "Bearer "+sendKey || idempotency != "alerta-1" {
		t.Fatalf("%s %s %s %s", method, path, auth, idempotency)
	}
	if accepted.Status != "sent" || accepted.DeliveryID != "4f3b0a0e-6a1c-4d2e-9b7a-1c2d3e4f5a6b" {
		t.Fatalf("%+v", accepted)
	}
	if strings.Contains(body, `"from"`) || strings.Contains(body, sendKey) || !strings.Contains(body, `"template":"usage.alert"`) {
		t.Fatal(body)
	}
	if !strings.Contains(body, `"channel":"email"`) || !strings.Contains(body, `"locale":"pt-BR"`) || !strings.Contains(body, `"used":"90"`) {
		t.Fatal(body)
	}
	status = http.StatusServiceUnavailable
	reply = `{"message":"fila ` + sendKey + `"}`
	_, err = SendMessage(context.Background(), server.Client(), server.URL, sendKey, "alerta-2", Message{
		To: "pessoa@exemplo.com", Template: TemplateKey,
	})
	if err == nil || strings.Contains(err.Error(), sendKey) {
		t.Fatal(err)
	}
	if !errors.As(err, &call) || call.Permanent() {
		t.Fatal("503 não pode ser final")
	}
}
