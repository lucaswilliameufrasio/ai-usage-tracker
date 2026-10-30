package login

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ai-usage-tracker/internal/access"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/token"
	"ai-usage-tracker/internal/vault"
)

const (
	sessionLength  = 14 * 24 * time.Hour
	attemptLength  = 10 * time.Minute
	appleSecretTTL = 5 * time.Minute
	maxUpstream    = 64 << 10
	userAgent      = "ai-usage-tracker"
)

var (
	ErrRejected    = errors.New("login: recusado")
	ErrUnavailable = errors.New("login: indisponível")
)

var googleIssuers = []string{"https://accounts.google.com", "accounts.google.com"}

var appleIssuer = "https://appleid.apple.com"

type Endpoints struct {
	GitHubAuthorize string
	GitHubToken     string
	GitHubUser      string
	GoogleAuthorize string
	GoogleToken     string
	GoogleJWKS      string
	AppleAuthorize  string
	AppleToken      string
	AppleJWKS       string
}

func Production() Endpoints {
	return Endpoints{
		GitHubAuthorize: "https://github.com/login/oauth/authorize",
		GitHubToken:     "https://github.com/login/oauth/access_token",
		GitHubUser:      "https://api.github.com/user",
		GoogleAuthorize: "https://accounts.google.com/o/oauth2/v2/auth",
		GoogleToken:     "https://oauth2.googleapis.com/token",
		GoogleJWKS:      "https://www.googleapis.com/oauth2/v3/certs",
		AppleAuthorize:  "https://appleid.apple.com/auth/authorize",
		AppleToken:      "https://appleid.apple.com/auth/token",
		AppleJWKS:       "https://appleid.apple.com/auth/keys",
	}
}

type Flow struct {
	Store *store.Store
	Vault vault.Box
	HTTP  *http.Client
	Ends  Endpoints
	Now   func() time.Time
}

func (f *Flow) Begin(ctx context.Context, provider string, redirectURI string) (string, error) {
	if !access.Known(provider) || redirectURI == "" {
		return "", ErrRejected
	}
	if f.Vault == nil {
		return "", ErrUnavailable
	}
	row, secret, err := f.currentSecret(ctx, provider)
	if err != nil {
		return "", err
	}
	defer clear(secret)
	ref, err := attemptSecretName()
	if err != nil {
		return "", err
	}
	if err := f.Vault.Put(ctx, ref, secret); err != nil {
		return "", ErrUnavailable
	}
	state, err := token.New("st_")
	if err != nil {
		_ = f.Vault.Delete(ctx, ref)
		return "", err
	}
	nonce, err := token.New("non_")
	if err != nil {
		_ = f.Vault.Delete(ctx, ref)
		return "", err
	}
	verifier, err := token.New("pkc_")
	if err != nil {
		_ = f.Vault.Delete(ctx, ref)
		return "", err
	}
	attemptID, err := token.New("att_")
	if err != nil {
		_ = f.Vault.Delete(ctx, ref)
		return "", err
	}
	now := f.now()
	attempt := store.LoginAttempt{
		ID:           attemptID,
		Provider:     provider,
		Nonce:        nonce,
		CodeVerifier: verifier,
		ClientID:     row.ClientID,
		IssuerURL:    row.IssuerURL,
		TeamID:       row.TeamID,
		KeyID:        row.KeyID,
		ServicesID:   row.ServicesID,
		SecretRef:    ref,
		RedirectURI:  redirectURI,
		ExpiresAt:    now.Add(attemptLength),
	}
	if err := f.Store.InsertAttempt(attempt, token.Hash(state)); err != nil {
		_ = f.Vault.Delete(ctx, ref)
		return "", err
	}
	location, err := f.authorizeURL(row, redirectURI, state, nonce, verifier)
	if err != nil {
		_ = f.Vault.Delete(ctx, ref)
		return "", err
	}
	return location, nil
}

func (f *Flow) Finish(ctx context.Context, provider string, code string, state string, redirectURI string) (string, error) {
	if code == "" || state == "" || !access.Known(provider) {
		return "", ErrRejected
	}
	if f.Vault == nil {
		return "", ErrUnavailable
	}
	attempt, err := f.Store.TakeAttempt(token.Hash(state), f.now())
	if attempt.SecretRef != "" {
		defer func() { _ = f.Vault.Delete(ctx, attempt.SecretRef) }()
	}
	if err != nil {
		return "", err
	}
	if attempt.Provider != provider || attempt.RedirectURI != redirectURI {
		return "", store.ErrLoginState
	}
	secret, err := f.Vault.Get(ctx, attempt.SecretRef)
	if err != nil {
		if errors.Is(err, vault.ErrNotFound) {
			return "", ErrRejected
		}
		return "", ErrUnavailable
	}
	defer clear(secret)
	subject, email, err := f.identify(ctx, attempt, secret, code)
	if err != nil {
		return "", err
	}
	accountID, err := token.New("acct_")
	if err != nil {
		return "", err
	}
	account, err := f.Store.UpsertAccount(provider, subject, email, accountID, f.now())
	if err != nil {
		return "", err
	}
	raw, err := token.New("ses_")
	if err != nil {
		return "", err
	}
	expires := f.now().Add(sessionLength)
	if err := f.Store.CreateSession(token.Hash(raw), account.ID, f.now(), expires); err != nil {
		return "", err
	}
	return raw, nil
}

func (f *Flow) currentSecret(ctx context.Context, provider string) (store.LoginProvider, []byte, error) {
	views, err := access.Catalog(ctx, f.Store, f.Vault)
	if err != nil {
		return store.LoginProvider{}, nil, ErrUnavailable
	}
	var chosen access.View
	found := false
	for _, view := range views {
		if view.Provider == provider {
			chosen = view
			found = true
			break
		}
	}
	if !found || !chosen.Enabled || !chosen.Configured || !chosen.SecretPresent {
		return store.LoginProvider{}, nil, ErrRejected
	}
	secret, err := f.Vault.Get(ctx, access.SecretRef(provider))
	if err != nil {
		if errors.Is(err, vault.ErrNotFound) {
			return store.LoginProvider{}, nil, ErrRejected
		}
		return store.LoginProvider{}, nil, ErrUnavailable
	}
	row := store.LoginProvider{
		Provider:   chosen.Provider,
		Enabled:    chosen.Enabled,
		ClientID:   chosen.ClientID,
		IssuerURL:  chosen.IssuerURL,
		TeamID:     chosen.TeamID,
		KeyID:      chosen.KeyID,
		ServicesID: chosen.ServicesID,
		SecretRef:  access.SecretRef(provider),
	}
	return row, secret, nil
}

func (f *Flow) authorizeURL(row store.LoginProvider, redirectURI string, state string, nonce string, verifier string) (string, error) {
	base, err := f.authorizeBase(row)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("client_id", clientID(row))
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge(verifier))
	query.Set("code_challenge_method", "S256")
	switch row.Provider {
	case access.GitHub:
		query.Set("scope", "read:user")
	case access.Gitea:
	case access.Google:
		query.Set("response_type", "code")
		query.Set("scope", "openid")
		query.Set("nonce", nonce)
	case access.Apple:
		query.Set("response_type", "code")
		query.Set("response_mode", "query")
		query.Set("scope", "openid")
		query.Set("nonce", nonce)
	default:
		return "", ErrRejected
	}
	if base == "" {
		return "", ErrRejected
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", ErrRejected
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (f *Flow) authorizeBase(row store.LoginProvider) (string, error) {
	switch row.Provider {
	case access.GitHub:
		return f.Ends.GitHubAuthorize, nil
	case access.Google:
		return f.Ends.GoogleAuthorize, nil
	case access.Apple:
		return f.Ends.AppleAuthorize, nil
	case access.Gitea:
		return endpoint(row.IssuerURL, "/login/oauth/authorize")
	default:
		return "", ErrRejected
	}
}

func (f *Flow) identify(ctx context.Context, attempt store.LoginAttempt, secret []byte, code string) (string, string, error) {
	switch attempt.Provider {
	case access.GitHub, access.Gitea:
		return f.userID(ctx, attempt, secret, code)
	case access.Google:
		return f.identityToken(ctx, attempt, secret, code, f.Ends.GoogleToken, f.Ends.GoogleJWKS, attempt.ClientID, googleIssuers)
	case access.Apple:
		return f.identityToken(ctx, attempt, secret, code, f.Ends.AppleToken, f.Ends.AppleJWKS, attempt.ServicesID, []string{appleIssuer})
	default:
		return "", "", ErrRejected
	}
}

func (f *Flow) userID(ctx context.Context, attempt store.LoginAttempt, secret []byte, code string) (string, string, error) {
	tokenURL, userURL, accept, err := f.userEndpoints(attempt)
	if err != nil {
		return "", "", err
	}
	form := url.Values{}
	form.Set("client_id", attempt.ClientID)
	form.Set("client_secret", string(secret))
	form.Set("code", code)
	form.Set("redirect_uri", attempt.RedirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", attempt.CodeVerifier)
	raw, err := f.postForm(ctx, tokenURL, form)
	if err != nil {
		return "", "", err
	}
	accessToken, err := stringField(raw, "access_token")
	if err != nil {
		return "", "", err
	}
	body, err := f.getBearer(ctx, userURL, accessToken, accept)
	if err != nil {
		return "", "", err
	}
	object, err := decodeObject(body)
	if err != nil {
		return "", "", err
	}
	subject, err := numberID(object["id"])
	if err != nil {
		return "", "", err
	}
	return subject, contactEmail(object["email"]), nil
}

func (f *Flow) userEndpoints(attempt store.LoginAttempt) (string, string, string, error) {
	if attempt.Provider == access.GitHub {
		return f.Ends.GitHubToken, f.Ends.GitHubUser, "application/vnd.github+json", nil
	}
	tokenURL, err := endpoint(attempt.IssuerURL, "/login/oauth/access_token")
	if err != nil {
		return "", "", "", err
	}
	userURL, err := endpoint(attempt.IssuerURL, "/api/v1/user")
	if err != nil {
		return "", "", "", err
	}
	return tokenURL, userURL, "application/json", nil
}

func (f *Flow) identityToken(ctx context.Context, attempt store.LoginAttempt, secret []byte, code string, tokenURL string, jwksURL string, audience string, issuers []string) (string, string, error) {
	clientSecret := string(secret)
	if attempt.Provider == access.Apple {
		minted, err := appleClientSecret(secret, attempt.TeamID, attempt.KeyID, attempt.ServicesID, f.now())
		if err != nil {
			return "", "", err
		}
		clientSecret = minted
	}
	form := url.Values{}
	form.Set("client_id", audience)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", attempt.RedirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", attempt.CodeVerifier)
	raw, err := f.postForm(ctx, tokenURL, form)
	if err != nil {
		return "", "", err
	}
	signed, err := stringField(raw, "id_token")
	if err != nil {
		return "", "", err
	}
	return f.verifyIDToken(ctx, signed, jwksURL, audience, attempt.Nonce, issuers)
}

func (f *Flow) verifyIDToken(ctx context.Context, signed string, jwksURL string, audience string, nonce string, issuers []string) (string, string, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(f.now),
	)
	claims := jwt.MapClaims{}
	parsed, err := parser.ParseWithClaims(signed, claims, func(header *jwt.Token) (any, error) {
		kid, _ := header.Header["kid"].(string)
		if kid == "" {
			return nil, ErrRejected
		}
		return f.rsaKey(ctx, jwksURL, kid)
	})
	if err != nil || parsed == nil || !parsed.Valid {
		return "", "", ErrRejected
	}
	issuer, _ := claims["iss"].(string)
	if !matchOne(issuer, issuers) {
		return "", "", ErrRejected
	}
	if !audienceMatches(claims["aud"], audience) {
		return "", "", ErrRejected
	}
	got, _ := claims["nonce"].(string)
	if !fixedText(got, nonce) {
		return "", "", ErrRejected
	}
	subject, _ := claims["sub"].(string)
	if subject == "" || len(subject) > 200 {
		return "", "", ErrRejected
	}
	return subject, contactEmail(claims["email"]), nil
}

func (f *Flow) rsaKey(ctx context.Context, jwksURL string, kid string) (*rsa.PublicKey, error) {
	raw, err := f.getJSON(ctx, jwksURL)
	if err != nil {
		return nil, ErrRejected
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, ErrRejected
	}
	for _, key := range set.Keys {
		if key.Kid != kid || key.Kty != "RSA" {
			continue
		}
		if key.Alg != "" && key.Alg != "RS256" {
			continue
		}
		if key.Use != "" && key.Use != "sig" {
			continue
		}
		public, err := key.public()
		if err != nil {
			return nil, ErrRejected
		}
		return public, nil
	}
	return nil, ErrRejected
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (key jwk) public() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil || len(nBytes) == 0 {
		return nil, ErrRejected
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil || len(eBytes) == 0 {
		return nil, ErrRejected
	}
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() {
		return nil, ErrRejected
	}
	value := e.Int64()
	if value < 2 || value > 1<<31-1 {
		return nil, ErrRejected
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(value)}, nil
}

func (f *Flow) postForm(ctx context.Context, endpointURL string, form url.Values) ([]byte, error) {
	if endpointURL == "" {
		return nil, ErrRejected
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, ErrRejected
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	return f.read(request)
}

func (f *Flow) getBearer(ctx context.Context, endpointURL string, accessToken string, accept string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return nil, ErrRejected
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", userAgent)
	return f.read(request)
}

func (f *Flow) getJSON(ctx context.Context, endpointURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return nil, ErrRejected
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	return f.read(request)
}

func (f *Flow) read(request *http.Request) ([]byte, error) {
	response, err := f.client().Do(request)
	if err != nil {
		return nil, ErrRejected
	}
	defer response.Body.Close()
	body, err := readLimited(response.Body, maxUpstream)
	if err != nil {
		return nil, ErrRejected
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrRejected
	}
	return body, nil
}

func (f *Flow) client() *http.Client {
	if f.HTTP != nil {
		return f.HTTP
	}
	return defaultClient
}

func (f *Flow) now() time.Time {
	if f.Now != nil {
		return f.Now().UTC()
	}
	return time.Now().UTC()
}

var defaultClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func appleClientSecret(raw []byte, teamID string, keyID string, servicesID string, now time.Time) (string, error) {
	private, err := parseP256(raw)
	if err != nil {
		return "", err
	}
	claims := jwt.RegisteredClaims{
		Issuer:    teamID,
		Subject:   servicesID,
		Audience:  jwt.ClaimStrings{appleIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(appleSecretTTL)),
	}
	signed := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	signed.Header["kid"] = keyID
	text, err := signed.SignedString(private)
	if err != nil {
		return "", ErrRejected
	}
	return text, nil
}

func parseP256(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, ErrRejected
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ec, ok := key.(*ecdsa.PrivateKey)
		if !ok || ec.Curve.Params().Name != "P-256" {
			return nil, ErrRejected
		}
		return ec, nil
	}
	ec, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil || ec.Curve.Params().Name != "P-256" {
		return nil, ErrRejected
	}
	return ec, nil
}

func attemptSecretName() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "login.attempt." + hex.EncodeToString(buf), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func clientID(row store.LoginProvider) string {
	if row.Provider == access.Apple && row.ServicesID != "" {
		return row.ServicesID
	}
	return row.ClientID
}

func endpoint(base string, path string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", ErrRejected
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", ErrRejected
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func decodeObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, ErrRejected
	}
	if object["error"] != nil {
		return nil, ErrRejected
	}
	return object, nil
}

func stringField(raw []byte, name string) (string, error) {
	object, err := decodeObject(raw)
	if err != nil {
		return "", err
	}
	value, _ := object[name].(string)
	if value == "" {
		return "", ErrRejected
	}
	return value, nil
}

func numberID(value any) (string, error) {
	number, ok := value.(json.Number)
	if !ok {
		return "", ErrRejected
	}
	text := number.String()
	if text == "" || len(text) > 40 {
		return "", ErrRejected
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return "", ErrRejected
		}
	}
	return text, nil
}

func contactEmail(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 200 || strings.ContainsAny(text, " \t\r\n") || !strings.Contains(text, "@") {
		return ""
	}
	return text
}

func audienceMatches(value any, want string) bool {
	switch audience := value.(type) {
	case string:
		return fixedText(audience, want)
	case []any:
		matched := false
		for _, item := range audience {
			text, ok := item.(string)
			if ok && fixedText(text, want) {
				matched = true
			}
		}
		return matched
	default:
		return false
	}
}

func matchOne(value string, allowed []string) bool {
	matched := false
	for _, item := range allowed {
		if fixedText(value, item) {
			matched = true
		}
	}
	return matched
}

func fixedText(got string, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("limite")
	}
	return raw, nil
}
