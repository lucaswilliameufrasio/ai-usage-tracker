package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// Ciphera reads and writes one key in a project environment.
// The documented read is the environment list. A prefix-scoped token
// drops the other keys before they are decrypted.
type Ciphera struct {
	Origin      string
	Project     string
	Environment string
	Token       string
	ServiceKey  string
	HTTP        *http.Client
}

type cipheraItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (c *Ciphera) Put(ctx context.Context, name string, plaintext []byte) error {
	if err := checkName(name); err != nil {
		return err
	}
	if err := checkPlain(plaintext); err != nil {
		return err
	}
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]any{
		"environment": c.Environment,
		"secrets": []map[string]string{{
			"key":   name,
			"value": string(plaintext),
		}},
	})
	if err != nil {
		return ErrUnavailable
	}
	status, _, err := c.call(ctx, token, http.MethodPost, c.projectPath("/secrets/batch"), raw)
	if err != nil {
		return err
	}
	return statusErr(status)
}

func (c *Ciphera) Get(ctx context.Context, name string) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	items, err := c.list(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Key == name {
			if len(item.Value) > maxPlain {
				return nil, ErrTooLarge
			}
			return []byte(item.Value), nil
		}
	}
	return nil, ErrNotFound
}

func (c *Ciphera) Delete(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}
	path := c.projectPath("/secrets/" + url.PathEscape(c.Environment) + "/" + url.PathEscape(name))
	status, _, err := c.call(ctx, token, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	return statusErr(status)
}

func (c *Ciphera) Present(ctx context.Context, name string) (bool, error) {
	if err := checkName(name); err != nil {
		return false, err
	}
	items, err := c.list(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Key == name {
			return true, nil
		}
	}
	return false, nil
}

func (c *Ciphera) list(ctx context.Context) ([]cipheraItem, error) {
	token, err := c.bearer(ctx)
	if err != nil {
		return nil, err
	}
	path := c.projectPath("/secrets/" + url.PathEscape(c.Environment))
	status, body, err := c.call(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if err := statusErr(status); err != nil {
		return nil, err
	}
	var items []cipheraItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, ErrSealed
	}
	return items, nil
}

func (c *Ciphera) bearer(ctx context.Context) (string, error) {
	if strings.TrimSpace(c.Token) != "" {
		return c.Token, nil
	}
	if strings.TrimSpace(c.ServiceKey) == "" {
		return "", ErrSealed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin()+"/v1/auth/service-account/token", nil)
	if err != nil {
		return "", ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.ServiceKey)
	status, body, err := do(ctx, c.HTTP, req)
	if err != nil {
		return "", err
	}
	if err := statusErr(status); err != nil {
		return "", err
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.AccessToken == "" {
		return "", ErrSealed
	}
	return parsed.AccessToken, nil
}

func (c *Ciphera) call(ctx context.Context, token string, method string, path string, raw []byte) (int, []byte, error) {
	var reader *bytes.Reader
	if raw != nil {
		reader = bytes.NewReader(raw)
	}
	var req *http.Request
	var err error
	if reader == nil {
		req, err = http.NewRequestWithContext(ctx, method, c.origin()+path, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.origin()+path, reader)
	}
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return do(ctx, c.HTTP, req)
}

func (c *Ciphera) projectPath(suffix string) string {
	return "/v1/projects/" + url.PathEscape(c.Project) + suffix
}

func (c *Ciphera) origin() string {
	return strings.TrimRight(c.Origin, "/")
}
