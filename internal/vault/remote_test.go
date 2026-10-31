package vault

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCipheraRoundTripUsesTheDocumentedRoutes(t *testing.T) {
	secret := "sintetico-ciphera"
	token := "st_sintetico"
	var store map[string]string
	store = map[string]string{}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls += 1
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/secrets/batch"):
			var body struct {
				Environment string `json:"environment"`
				Secrets     []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"secrets"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Environment != "dev" || len(body.Secrets) != 1 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			store[body.Secrets[0].Key] = body.Secrets[0].Value
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/secrets/dev"):
			items := make([]map[string]string, 0, len(store))
			for key, value := range store {
				items = append(items, map[string]string{"key": key, "value": value})
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/secrets/dev/"):
			name := strings.TrimPrefix(r.URL.Path, "/v1/projects/proj/secrets/dev/")
			delete(store, name)
			_ = json.NewEncoder(w).Encode(map[string]any{"key": name, "is_deleted": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	box := &Ciphera{
		Origin:      server.URL,
		Project:     "proj",
		Environment: "dev",
		Token:       token,
		HTTP:        server.Client(),
	}
	ctx := context.Background()
	if err := box.Put(ctx, "login.github", []byte(secret)); err != nil {
		t.Fatal(err)
	}
	got, err := box.Get(ctx, "login.github")
	if err != nil || string(got) != secret {
		t.Fatal(err)
	}
	present, err := box.Present(ctx, "login.github")
	if err != nil || !present {
		t.Fatal(err)
	}
	if err := box.Delete(ctx, "login.github"); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Get(ctx, "login.github"); err != ErrNotFound {
		t.Fatal(err)
	}
	if strings.Contains(ErrNotFound.Error(), secret) || calls == 0 {
		t.Fatal("vazou ou não chamou")
	}
}

func TestCipheraAuthFailureStaysClosed(t *testing.T) {
	secret := "sintetico-nao-mostrar"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, secret, http.StatusForbidden)
	}))
	defer server.Close()
	box := &Ciphera{Origin: server.URL, Project: "proj", Environment: "dev", Token: "st_sintetico", HTTP: server.Client()}
	_, err := box.Get(context.Background(), "login.github")
	if err != ErrSealed || strings.Contains(err.Error(), secret) {
		t.Fatal(err)
	}
}

func TestCipheraRejectsOversizedBeforeTheRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()
	box := &Ciphera{Origin: server.URL, Project: "proj", Environment: "dev", Token: "st_sintetico", HTTP: server.Client()}
	err := box.Put(context.Background(), "login.github", bytesOf(maxPlain+1))
	if err != ErrTooLarge || called {
		t.Fatal(err)
	}
}

func TestCipheraServiceKeyIsExchangedBeforeTheRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/service-account/token" {
			if r.Header.Get("Authorization") != "Bearer sak_sintetico" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access_sintetico"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer access_sintetico" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer server.Close()
	box := &Ciphera{Origin: server.URL, Project: "proj", Environment: "dev", ServiceKey: "sak_sintetico", HTTP: server.Client()}
	present, err := box.Present(context.Background(), "login.github")
	if err != nil || present {
		t.Fatal(err)
	}
}

func TestHashiCorpKVRoundTrip(t *testing.T) {
	secret := "sintetico-vault"
	stored := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "hvs_sintetico" || r.Header.Get("X-Vault-Namespace") != "ns" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != "/v1/secret/data/ai/login.github" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodPost:
			var body struct {
				Data map[string]string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			stored = body.Data["value"]
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if stored == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": map[string]string{"value": stored}}})
		case http.MethodDelete:
			stored = ""
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	box := &HashiCorp{
		Address:   server.URL,
		Token:     "hvs_sintetico",
		Prefix:    "ai",
		Namespace: "ns",
		HTTP:      server.Client(),
	}
	ctx := context.Background()
	if err := box.Put(ctx, "login.github", []byte(secret)); err != nil {
		t.Fatal(err)
	}
	got, err := box.Get(ctx, "login.github")
	if err != nil || string(got) != secret {
		t.Fatal(err)
	}
	if err := box.Delete(ctx, "login.github"); err != nil {
		t.Fatal(err)
	}
	present, err := box.Present(ctx, "login.github")
	if err != nil || present {
		t.Fatal(err)
	}
}

func TestSecretsManagerRoundTrip(t *testing.T) {
	secret := "sintetico-aws"
	stored := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("X-Amz-Security-Token") != "session_sintetico" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		target := r.Header.Get("X-Amz-Target")
		switch target {
		case "secretsmanager.CreateSecret":
			if stored != "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"__type":"ResourceExistsException"}`))
				return
			}
			var body struct {
				SecretString string `json:"SecretString"`
			}
			_ = json.Unmarshal(raw, &body)
			stored = body.SecretString
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ARN":"arn:aws:secretsmanager:us-east-1:000000000000:secret:login.github"}`))
		case "secretsmanager.GetSecretValue":
			if stored == "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"__type":"ResourceNotFoundException"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"SecretString": stored})
		case "secretsmanager.PutSecretValue":
			var body struct {
				SecretString string `json:"SecretString"`
			}
			_ = json.Unmarshal(raw, &body)
			stored = body.SecretString
			w.WriteHeader(http.StatusOK)
		case "secretsmanager.DeleteSecret":
			if !strings.Contains(string(raw), `"ForceDeleteWithoutRecovery":true`) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			stored = ""
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ARN":"arn:aws:secretsmanager:us-east-1:000000000000:secret:login.github"}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	box := &SecretsManager{
		Region:          "us-east-1",
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "sintetico-segredo-aws",
		SessionToken:    "session_sintetico",
		Endpoint:        server.URL,
		HTTP:            server.Client(),
		Now:             func() time.Time { return time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC) },
	}
	ctx := context.Background()
	if err := box.Put(ctx, "login.github", []byte(secret)); err != nil {
		t.Fatal(err)
	}
	if err := box.Put(ctx, "login.github", []byte(secret+"-2")); err != nil {
		t.Fatal(err)
	}
	got, err := box.Get(ctx, "login.github")
	if err != nil || string(got) != secret+"-2" {
		t.Fatal(err)
	}
	if err := box.Delete(ctx, "login.github"); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Get(ctx, "login.github"); err != ErrNotFound || strings.Contains(err.Error(), secret) {
		t.Fatal(err)
	}
}

func TestInfisicalRoundTripExchangesTheMachineIdentity(t *testing.T) {
	secret := "sintetico-infisical"
	stored := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/universal-auth/login" {
			var body struct {
				ClientID     string `json:"clientId"`
				ClientSecret string `json:"clientSecret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ClientID != "id_sintetico" || body.ClientSecret != "sec_sintetico" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "access_sintetico", "expiresIn": 60})
			return
		}
		if r.Header.Get("Authorization") != "Bearer access_sintetico" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v4/secrets/login.github" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if stored == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.URL.Query().Get("viewSecretValue") != "true" || r.URL.Query().Get("projectId") != "proj" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"secret": map[string]string{"secretValue": stored}})
		case http.MethodPost:
			if stored != "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var body struct {
				SecretValue string `json:"secretValue"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			stored = body.SecretValue
			_ = json.NewEncoder(w).Encode(map[string]any{"secret": map[string]string{"secretValue": stored}})
		case http.MethodPatch:
			var body struct {
				SecretValue string `json:"secretValue"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			stored = body.SecretValue
			_ = json.NewEncoder(w).Encode(map[string]any{"secret": map[string]string{"secretValue": stored}})
		case http.MethodDelete:
			stored = ""
			_ = json.NewEncoder(w).Encode(map[string]any{"secret": map[string]string{"secretKey": "login.github"}})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	box := &Infisical{
		Origin:       server.URL,
		ProjectID:    "proj",
		Environment:  "dev",
		ClientID:     "id_sintetico",
		ClientSecret: "sec_sintetico",
		HTTP:         server.Client(),
	}
	ctx := context.Background()
	if err := box.Put(ctx, "login.github", []byte(secret)); err != nil {
		t.Fatal(err)
	}
	if err := box.Put(ctx, "login.github", []byte(secret+"-2")); err != nil {
		t.Fatal(err)
	}
	got, err := box.Get(ctx, "login.github")
	if err != nil || string(got) != secret+"-2" {
		t.Fatal(err)
	}
	if err := box.Delete(ctx, "login.github"); err != nil {
		t.Fatal(err)
	}
	present, err := box.Present(ctx, "login.github")
	if err != nil || present {
		t.Fatal(err)
	}
}

func TestInfisicalApprovalDoesNotCountAsStored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "universal-auth") {
			_ = json.NewEncoder(w).Encode(map[string]string{"accessToken": "access_sintetico"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"approval": map[string]string{"id": "apr"}})
	}))
	defer server.Close()
	box := &Infisical{Origin: server.URL, ProjectID: "proj", Environment: "dev", ClientID: "id", ClientSecret: "sec", HTTP: server.Client()}
	err := box.Put(context.Background(), "login.github", []byte("sintetico"))
	if err != ErrUnavailable {
		t.Fatal(err)
	}
}

func TestOpenConfiguredBuildsOnlyCompleteBackends(t *testing.T) {
	env := map[string]string{
		"TRACKER_CIPHERA_ORIGIN":      "http://127.0.0.1:9",
		"TRACKER_CIPHERA_PROJECT":     "proj",
		"TRACKER_CIPHERA_ENVIRONMENT": "dev",
		"TRACKER_HASHICORP_ADDR":      "http://127.0.0.1:9",
	}
	boxes := OpenConfigured(func(key string) string { return env[key] }, t.TempDir())
	if len(boxes) != 0 {
		t.Fatalf("abriu %d", len(boxes))
	}
	env["TRACKER_CIPHERA_TOKEN"] = "st_sintetico"
	boxes = OpenConfigured(func(key string) string { return env[key] }, t.TempDir())
	if _, ok := boxes[KindCiphera]; !ok || len(boxes) != 1 {
		t.Fatalf("ciphera %d", len(boxes))
	}
}

func bytesOf(n int) []byte {
	return []byte(strings.Repeat("a", n))
}
