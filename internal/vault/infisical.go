package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// Infisical uses a machine identity. The client secret stays in the
// process opening config and is exchanged on each call.
type Infisical struct {
	Origin       string
	ProjectID    string
	Environment  string
	SecretPath   string
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
}

func (n *Infisical) Put(ctx context.Context, name string, plaintext []byte) error {
	if err := checkName(name); err != nil {
		return err
	}
	if err := checkPlain(plaintext); err != nil {
		return err
	}
	token, err := n.login(ctx)
	if err != nil {
		return err
	}
	body := map[string]string{
		"projectId":   n.ProjectID,
		"environment": n.Environment,
		"secretPath":  n.path(),
		"secretValue": string(plaintext),
		"type":        "shared",
	}
	status, raw, err := n.call(ctx, token, http.MethodPost, n.secretPath(name), body)
	if err != nil {
		return err
	}
	if status == http.StatusBadRequest {
		status, raw, err = n.call(ctx, token, http.MethodPatch, n.secretPath(name), body)
		if err != nil {
			return err
		}
	}
	if err := statusErr(status); err != nil {
		return err
	}
	return requireSecret(raw)
}

func (n *Infisical) Get(ctx context.Context, name string) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	token, err := n.login(ctx)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("projectId", n.ProjectID)
	query.Set("environment", n.Environment)
	query.Set("secretPath", n.path())
	query.Set("type", "shared")
	query.Set("viewSecretValue", "true")
	status, raw, err := n.call(ctx, token, http.MethodGet, n.secretPath(name)+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if err := statusErr(status); err != nil {
		return nil, err
	}
	var parsed struct {
		Secret struct {
			SecretValue string `json:"secretValue"`
		} `json:"secret"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Secret.SecretValue == "" {
		return nil, ErrSealed
	}
	if len(parsed.Secret.SecretValue) > maxPlain {
		return nil, ErrTooLarge
	}
	return []byte(parsed.Secret.SecretValue), nil
}

func (n *Infisical) Delete(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	token, err := n.login(ctx)
	if err != nil {
		return err
	}
	body := map[string]string{
		"projectId":   n.ProjectID,
		"environment": n.Environment,
		"secretPath":  n.path(),
		"type":        "shared",
	}
	status, raw, err := n.call(ctx, token, http.MethodDelete, n.secretPath(name), body)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if err := statusErr(status); err != nil {
		return err
	}
	return requireSecret(raw)
}

func (n *Infisical) Present(ctx context.Context, name string) (bool, error) {
	_, err := n.Get(ctx, name)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (n *Infisical) login(ctx context.Context) (string, error) {
	raw, err := json.Marshal(map[string]string{
		"clientId":     n.ClientID,
		"clientSecret": n.ClientSecret,
	})
	if err != nil {
		return "", ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.origin()+"/api/v1/auth/universal-auth/login", bytes.NewReader(raw))
	if err != nil {
		return "", ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	status, body, err := do(ctx, n.HTTP, req)
	if err != nil {
		return "", err
	}
	if err := statusErr(status); err != nil {
		return "", err
	}
	var parsed struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.AccessToken == "" {
		return "", ErrSealed
	}
	return parsed.AccessToken, nil
}

func (n *Infisical) call(ctx context.Context, token string, method string, path string, payload any) (int, []byte, error) {
	var raw []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, ErrUnavailable
		}
		raw = encoded
	}
	var req *http.Request
	var err error
	if raw == nil {
		req, err = http.NewRequestWithContext(ctx, method, n.origin()+path, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, n.origin()+path, bytes.NewReader(raw))
	}
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return do(ctx, n.HTTP, req)
}

func (n *Infisical) secretPath(name string) string {
	return "/api/v4/secrets/" + url.PathEscape(name)
}

func (n *Infisical) path() string {
	if strings.TrimSpace(n.SecretPath) == "" {
		return "/"
	}
	return n.SecretPath
}

func (n *Infisical) origin() string {
	return strings.TrimRight(n.Origin, "/")
}

func requireSecret(raw []byte) error {
	var parsed struct {
		Secret   *json.RawMessage `json:"secret"`
		Approval *json.RawMessage `json:"approval"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ErrSealed
	}
	if parsed.Secret == nil || parsed.Approval != nil {
		return ErrUnavailable
	}
	return nil
}
