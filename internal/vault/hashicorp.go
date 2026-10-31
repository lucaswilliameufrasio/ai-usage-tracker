package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// HashiCorp is a KV version 2 client. Transit is not this port.
type HashiCorp struct {
	Address   string
	Token     string
	Mount     string
	Prefix    string
	Namespace string
	HTTP      *http.Client
}

func (h *HashiCorp) Put(ctx context.Context, name string, plaintext []byte) error {
	if err := checkName(name); err != nil {
		return err
	}
	if err := checkPlain(plaintext); err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]any{
		"data": map[string]string{"value": string(plaintext)},
	})
	if err != nil {
		return ErrUnavailable
	}
	status, _, err := h.call(ctx, http.MethodPost, h.dataPath(name), raw)
	if err != nil {
		return err
	}
	return statusErr(status)
}

func (h *HashiCorp) Get(ctx context.Context, name string) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	status, body, err := h.call(ctx, http.MethodGet, h.dataPath(name), nil)
	if err != nil {
		return nil, err
	}
	if err := statusErr(status); err != nil {
		return nil, err
	}
	var parsed struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, ErrSealed
	}
	value, ok := parsed.Data.Data["value"]
	if !ok {
		return nil, ErrSealed
	}
	if len(value) > maxPlain {
		return nil, ErrTooLarge
	}
	return []byte(value), nil
}

func (h *HashiCorp) Delete(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	status, _, err := h.call(ctx, http.MethodDelete, h.dataPath(name), nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	return statusErr(status)
}

func (h *HashiCorp) Present(ctx context.Context, name string) (bool, error) {
	_, err := h.Get(ctx, name)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (h *HashiCorp) call(ctx context.Context, method string, path string, raw []byte) (int, []byte, error) {
	var req *http.Request
	var err error
	if raw == nil {
		req, err = http.NewRequestWithContext(ctx, method, h.origin()+path, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, h.origin()+path, bytes.NewReader(raw))
	}
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	req.Header.Set("X-Vault-Token", h.Token)
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(h.Namespace) != "" {
		req.Header.Set("X-Vault-Namespace", h.Namespace)
	}
	return do(ctx, h.HTTP, req)
}

func (h *HashiCorp) dataPath(name string) string {
	mount := h.Mount
	if mount == "" {
		mount = "secret"
	}
	tail := name
	prefix := strings.Trim(h.Prefix, "/")
	if prefix != "" {
		tail = prefix + "/" + name
	}
	return "/v1/" + escapeSegment(mount) + "/data/" + escapePath(tail)
}

func (h *HashiCorp) origin() string {
	return strings.TrimRight(h.Address, "/")
}

func escapePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = escapeSegment(part)
	}
	return strings.Join(parts, "/")
}

func escapeSegment(part string) string {
	return url.PathEscape(part)
}
