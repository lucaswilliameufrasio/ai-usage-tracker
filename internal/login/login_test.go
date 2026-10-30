package login

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ai-usage-tracker/internal/access"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

func TestGiteaUsesNumericIDAndKeepsTheAttempt(t *testing.T) {
	const secret = "sintetico-segredo-gitea"
	const subject = "9007199254740993"
	var mu sync.Mutex
	seen := ""
	calls := 0
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/oauth/access_token" {
			body := readBody(t, r)
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Errorf("form")
				return
			}
			mu.Lock()
			seen = form.Get("client_secret")
			calls++
			mu.Unlock()
			writeJSONText(w, `{"access_token":"token-falso"}`)
			return
		}
		if r.URL.Path == "/api/v1/user" {
			if strings.Contains(r.Header.Get("Authorization"), secret) {
				t.Errorf("usuário recebeu o segredo")
			}
			writeJSONText(w, `{"id":`+subject+`}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(idp.Close)
	flow, db, dbPath, vaultPath := openFlow(t, Endpoints{}, fixedNow)
	saveLogin(t, db, flow.Vault, access.Draft{
		Provider:  access.Gitea,
		Enabled:   true,
		ClientID:  "id-gitea",
		IssuerURL: idp.URL,
		Secret:    bytesOf(secret),
		Now:       fixedNow,
	})
	redirect := "http://tracker.test/v1/login/gitea/callback"
	location := begin(t, flow, access.Gitea, redirect)
	if strings.Contains(location, secret) || strings.Contains(location, "scope=") {
		t.Fatal("autorização")
	}
	state := mustQuery(t, location, "state")
	if _, err := flow.Finish(context.Background(), access.Gitea, "codigo-sintetico", state, "http://errado.test/callback"); err != store.ErrLoginState {
		t.Fatal("retorno trocado")
	}
	if calls != 0 {
		t.Fatal("troca antecipada")
	}
	location = begin(t, flow, access.Gitea, redirect)
	session, err := flow.Finish(context.Background(), access.Gitea, "codigo-sintetico", mustQuery(t, location, "state"), redirect)
	if err != nil || !strings.HasPrefix(session, "ses_") {
		t.Fatal("sessão")
	}
	if seen != secret {
		t.Fatal("segredo da tentativa")
	}
	account, err := db.FindAccount(access.Gitea, subject)
	if err != nil || account.Subject != subject {
		t.Fatal("sujeito")
	}
	assertFileAbsent(t, dbPath, secret)
	if countFiles(t, vaultPath) != 1 {
		t.Fatal("cópia da tentativa")
	}
}

func TestGoogleChecksIDToken(t *testing.T) {
	const secret = "sintetico-segredo-google"
	const clientID = "id-google.apps.googleusercontent.com"
	key := rsaKey(t)
	var mu sync.Mutex
	tokenBody := ""
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			mu.Lock()
			body := tokenBody
			mu.Unlock()
			writeJSONText(w, body)
			return
		}
		if r.URL.Path == "/certs" {
			writeJSONText(w, jwks(key))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(idp.Close)
	flow, db, dbPath, vaultPath := openFlow(t, Endpoints{
		GoogleAuthorize: idp.URL + "/authorize",
		GoogleToken:     idp.URL + "/token",
		GoogleJWKS:      idp.URL + "/certs",
	}, fixedNow)
	saveLogin(t, db, flow.Vault, access.Draft{
		Provider: access.Google,
		Enabled:  true,
		ClientID: clientID,
		Secret:   bytesOf(secret),
		Now:      fixedNow,
	})
	redirect := "http://tracker.test/v1/login/google/callback"
	finish := func(subject string, issuer string, email string, nonceMode string) error {
		location := begin(t, flow, access.Google, redirect)
		nonce := mustQuery(t, location, "nonce")
		if nonceMode == "errado" {
			nonce += "x"
		}
		claims := jwt.MapClaims{
			"iss":   issuer,
			"aud":   clientID,
			"sub":   subject,
			"nonce": nonce,
			"iat":   fixedNow.Unix(),
			"exp":   fixedNow.Add(time.Hour).Unix(),
		}
		if email != "" {
			claims["email"] = email
		}
		signed := signRSA(t, key, claims)
		mu.Lock()
		tokenBody = `{"id_token":"` + signed + `"}`
		mu.Unlock()
		_, err := flow.Finish(context.Background(), access.Google, "codigo-sintetico", mustQuery(t, location, "state"), redirect)
		return err
	}
	if err := finish("google-sub-1", "https://accounts.google.com", "pessoa@exemplo.test", "certo"); err != nil {
		t.Fatal(err)
	}
	account, err := db.FindAccount(access.Google, "google-sub-1")
	if err != nil || account.Contact != "pessoa@exemplo.test" {
		t.Fatal("contato")
	}
	if err := finish("google-sub-1", "accounts.google.com", "", "certo"); err != nil {
		t.Fatal(err)
	}
	account, err = db.FindAccount(access.Google, "google-sub-1")
	if err != nil || account.Contact != "pessoa@exemplo.test" {
		t.Fatal("contato apagado")
	}
	if err := finish("google-sub-1", "https://accounts.google.com", "", "errado"); err == nil {
		t.Fatal("nonce")
	}
	if err := finish("google-sub-2", "https://evil.example", "", "certo"); err == nil {
		t.Fatal("emissor")
	}
	location := begin(t, flow, access.Google, redirect)
	mu.Lock()
	tokenBody = `{"id_token":"` + unsignedToken(mustQuery(t, location, "nonce"), clientID) + `"}`
	mu.Unlock()
	if _, err := flow.Finish(context.Background(), access.Google, "codigo-sintetico", mustQuery(t, location, "state"), redirect); err == nil {
		t.Fatal("alg none")
	}
	assertFileAbsent(t, dbPath, secret)
	if countFiles(t, vaultPath) != 1 {
		t.Fatal("cópia da tentativa")
	}
}

func TestAppleClientSecretLastsFiveMinutes(t *testing.T) {
	private, rawPEM := p256PEM(t)
	rsaPrivate := rsaKey(t)
	var mu sync.Mutex
	clientSecret := ""
	idToken := ""
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			form, err := url.ParseQuery(string(readBody(t, r)))
			if err != nil {
				t.Errorf("form")
				return
			}
			mu.Lock()
			clientSecret = form.Get("client_secret")
			body := idToken
			mu.Unlock()
			writeJSONText(w, `{"id_token":"`+body+`"}`)
			return
		}
		if r.URL.Path == "/auth/keys" {
			writeJSONText(w, jwks(rsaPrivate))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(idp.Close)
	flow, db, dbPath, vaultPath := openFlow(t, Endpoints{
		AppleAuthorize: idp.URL + "/auth/authorize",
		AppleToken:     idp.URL + "/auth/token",
		AppleJWKS:      idp.URL + "/auth/keys",
	}, fixedNow)
	saveLogin(t, db, flow.Vault, access.Draft{
		Provider:   access.Apple,
		Enabled:    true,
		TeamID:     "TEAMID1234",
		KeyID:      "KEYID12345",
		ServicesID: "com.exemplo.teste",
		Secret:     &rawPEM,
		Now:        fixedNow,
	})
	redirect := "http://tracker.test/v1/login/apple/callback"
	location := begin(t, flow, access.Apple, redirect)
	if !strings.Contains(location, "response_mode=query") || strings.Contains(location, "BEGIN PRIVATE KEY") {
		t.Fatal("autorização")
	}
	nonce := mustQuery(t, location, "nonce")
	mu.Lock()
	idToken = signRSA(t, rsaPrivate, jwt.MapClaims{
		"iss":   "https://appleid.apple.com",
		"aud":   "com.exemplo.teste",
		"sub":   "apple-sub-1",
		"nonce": nonce,
		"iat":   fixedNow.Unix(),
		"exp":   fixedNow.Add(time.Hour).Unix(),
		"email": "nao e email",
	})
	mu.Unlock()
	session, err := flow.Finish(context.Background(), access.Apple, "codigo-sintetico", mustQuery(t, location, "state"), redirect)
	if err != nil || !strings.HasPrefix(session, "ses_") {
		t.Fatal("sessão")
	}
	if strings.Contains(clientSecret, "BEGIN PRIVATE KEY") || clientSecret == "" {
		t.Fatal("client secret")
	}
	claims := jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(clientSecret, &claims, func(token *jwt.Token) (any, error) {
		if token.Header["kid"] != "KEYID12345" {
			return nil, errKid
		}
		return &private.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}), jwt.WithTimeFunc(func() time.Time { return fixedNow }))
	if err != nil || !parsed.Valid || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("assinatura")
	}
	delta := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if delta != 5*time.Minute || delta > 15777000*time.Second {
		t.Fatal("validade")
	}
	if claims.Issuer != "TEAMID1234" || claims.Subject != "com.exemplo.teste" {
		t.Fatal("claims")
	}
	account, err := db.FindAccount(access.Apple, "apple-sub-1")
	if err != nil || account.Contact != "" {
		t.Fatal("contato")
	}
	assertFileAbsent(t, dbPath, "BEGIN PRIVATE KEY", clientSecret)
	if countFiles(t, vaultPath) != 1 {
		t.Fatal("cópia da tentativa")
	}
}

func openFlow(t *testing.T, ends Endpoints, now time.Time) (*Flow, *store.Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracker.db")
	vaultPath := filepath.Join(dir, "vault")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := vault.OpenOwn(vaultPath, bytesOfKey())
	if err != nil {
		t.Fatal(err)
	}
	flow := &Flow{
		Store: db,
		Vault: box,
		HTTP: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		Ends: ends,
		Now:  func() time.Time { return now },
	}
	return flow, db, dbPath, vaultPath
}

func saveLogin(t *testing.T, db *store.Store, box vault.Box, draft access.Draft) {
	t.Helper()
	if err := access.Save(context.Background(), db, box, draft); err != nil {
		t.Fatal(err)
	}
}

func begin(t *testing.T, flow *Flow, provider string, redirect string) string {
	t.Helper()
	location, err := flow.Begin(context.Background(), provider, redirect)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func mustQuery(t *testing.T, location string, name string) string {
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

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func p256PEM(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
}

func jwks(key *rsa.PrivateKey) string {
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	return `{"keys":[{"kty":"RSA","kid":"chave-teste","alg":"RS256","use":"sig","n":"` + n + `","e":"` + e + `"}]}`
}

func signRSA(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	signed := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed.Header["kid"] = "chave-teste"
	text, err := signed.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func unsignedToken(nonce string, audience string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"chave-teste"}`))
	payload, err := json.Marshal(map[string]any{
		"iss":   "https://accounts.google.com",
		"aud":   audience,
		"sub":   "none-sub",
		"nonce": nonce,
		"exp":   fixedNow.Add(time.Hour).Unix(),
		"iat":   fixedNow.Unix(),
	})
	if err != nil {
		return ""
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeJSONText(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func bytesOf(value string) *[]byte {
	raw := []byte(value)
	return &raw
}

func bytesOfKey() []byte {
	return []byte{
		1, 2, 3, 4, 5, 6, 7, 8,
		1, 2, 3, 4, 5, 6, 7, 8,
		1, 2, 3, 4, 5, 6, 7, 8,
		1, 2, 3, 4, 5, 6, 7, 8,
	}
}

func assertFileAbsent(t *testing.T, path string, banned ...string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range banned {
		if strings.Contains(string(raw), item) {
			t.Fatalf("%s guarda segredo", filepath.Base(path))
		}
	}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

var errKid = errors.New("kid")

var fixedNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
