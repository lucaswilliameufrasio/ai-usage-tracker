package sabia

import (
	"context"
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
