package mail

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSigV4MatchesThePublishedIAMExample(t *testing.T) {
	payloadHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if len(payloadHash) != 64 {
		t.Fatal(payloadHash)
	}
	got := signV4(
		http.MethodGet,
		"/",
		"Action=ListUsers&Version=2010-05-08",
		"iam.amazonaws.com",
		"20150830T123600Z",
		"us-east-1",
		"iam",
		"AKIDEXAMPLE",
		"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		nil,
		map[string]string{
			"content-type": "application/x-www-form-urlencoded; charset=utf-8",
			"host":         "iam.amazonaws.com",
			"x-amz-date":   "20150830T123600Z",
		},
	)
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/iam/aws4_request, SignedHeaders=content-type;host;x-amz-date, Signature=5d672d79c15b13162d9279b0855cfba6789a8edb4c82c400e06b5924a6f2b5d7"
	if got != want {
		t.Fatal(got)
	}
}

func TestProvidersUseTheDocumentedPath(t *testing.T) {
	secret := "re_sintetico_nao_usar"
	ms := "ms_sintetico_nao_usar"
	cf := "cf_sintetico_nao_usar"
	sesSecret := "ses_sintetico_nao_usar"
	cases := []struct {
		provider string
		key      string
		path     string
		account  string
	}{
		{provider: ProviderResend, key: secret, path: "/emails"},
		{provider: ProviderMillionSend, key: ms, path: "/emails"},
		{provider: ProviderCloudflare, key: cf, path: "/client/v4/accounts/conta123/email/sending/send", account: "conta123"},
		{provider: ProviderSES, key: sesSecret, path: "/v2/email/outbound-emails", account: "us-east-1"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			var seen *http.Request
			var body []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = r.Clone(r.Context())
				raw, _ := io.ReadAll(r.Body)
				body = raw
				if tc.provider == ProviderCloudflare {
					_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":{"delivered":[],"permanent_bounces":[],"queued":["pessoa@exemplo.com"]}}`))
					return
				}
				if tc.provider == ProviderSES {
					_, _ = w.Write([]byte(`{"MessageId":"msg_sintetico"}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":"msg_sintetico"}`))
			}))
			t.Cleanup(server.Close)
			client := &Client{
				HTTP:        server.Client(),
				Resend:      server.URL,
				MillionSend: server.URL,
				Cloudflare:  server.URL,
				SESBase:     server.URL,
				Now:         func() time.Time { return time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC) },
			}
			creds := Credentials{APIKey: tc.key}
			if tc.provider == ProviderSES {
				creds = Credentials{AccessKeyID: "AKIAEXEMPLO", SecretAccessKey: tc.key}
			}
			err := client.Send(context.Background(), tc.provider, tc.account, creds, Message{
				From: "Avisos <avisos@exemplo.com>", To: "pessoa@exemplo.com", Subject: "Limiar de uso", Text: "corpo",
			})
			if err != nil {
				t.Fatal(err)
			}
			if seen == nil || seen.Method != http.MethodPost || seen.URL.Path != tc.path {
				t.Fatalf("%v", seen)
			}
			if strings.Contains(string(body), tc.key) || strings.Contains(seen.URL.String(), tc.key) {
				t.Fatal("chave foi parar no corpo ou na url")
			}
			if tc.provider == ProviderSES {
				auth := seen.Header.Get("Authorization")
				if !strings.Contains(auth, "Credential=AKIAEXEMPLO/20150830/us-east-1/ses/aws4_request") || strings.Contains(auth, tc.key) {
					t.Fatal(auth)
				}
				var payload map[string]any
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["FromEmailAddress"] != "Avisos <avisos@exemplo.com>" {
					t.Fatalf("%v", payload["FromEmailAddress"])
				}
				return
			}
			if seen.Header.Get("Authorization") != "Bearer "+tc.key {
				t.Fatal(seen.Header.Get("Authorization"))
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["from"] != "Avisos <avisos@exemplo.com>" || payload["subject"] != "Limiar de uso" || payload["text"] != "corpo" {
				t.Fatalf("%s", body)
			}
		})
	}
}

func TestStatusClassifiesRetryAndRejection(t *testing.T) {
	if !errors.Is(classify(http.StatusTooManyRequests), ErrRetry) || !errors.Is(classify(http.StatusBadGateway), ErrRetry) {
		t.Fatal("transitório")
	}
	if !errors.Is(classify(http.StatusBadRequest), ErrPermanent) || !errors.Is(classify(http.StatusUnauthorized), ErrPermanent) {
		t.Fatal("definitivo")
	}
}

func TestCloudflareFalseSuccessIsFinal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10001}],"messages":[],"result":null}`))
	}))
	t.Cleanup(server.Close)
	client := &Client{HTTP: server.Client(), Cloudflare: server.URL}
	err := client.Send(context.Background(), ProviderCloudflare, "conta123", Credentials{APIKey: "cf_sintetico"}, Message{
		From: "avisos@exemplo.com", To: "pessoa@exemplo.com", Subject: "s", Text: "t",
	})
	if !errors.Is(err, ErrPermanent) {
		t.Fatal(err)
	}
}
