package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ProviderResend      = "resend"
	ProviderMillionSend = "millionsend"
	ProviderCloudflare  = "cloudflare"
	ProviderSES         = "ses"
	SecretName          = "email.send"
)

var (
	ErrRetry     = errors.New("mail: tentar de novo")
	ErrPermanent = errors.New("mail: definitivo")
)

type Credentials struct {
	APIKey          string
	AccessKeyID     string
	SecretAccessKey string
}

type Message struct {
	From    string
	To      string
	Subject string
	Text    string
}

type Client struct {
	HTTP        *http.Client
	Resend      string
	MillionSend string
	Cloudflare  string
	SESBase     string
	Now         func() time.Time
}

func Default() *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		Resend:      "https://api.resend.com",
		MillionSend: "https://api.millionsend.com",
		Cloudflare:  "https://api.cloudflare.com",
		Now:         func() time.Time { return time.Now().UTC() },
	}
}

func (c *Client) Send(ctx context.Context, provider string, accountRef string, creds Credentials, msg Message) error {
	switch provider {
	case ProviderResend:
		return c.postJSON(ctx, strings.TrimRight(c.base(c.Resend, "https://api.resend.com"), "/")+"/emails", creds.APIKey, resendBody(msg))
	case ProviderMillionSend:
		return c.postJSON(ctx, strings.TrimRight(c.base(c.MillionSend, "https://api.millionsend.com"), "/")+"/emails", creds.APIKey, resendBody(msg))
	case ProviderCloudflare:
		endpoint := strings.TrimRight(c.base(c.Cloudflare, "https://api.cloudflare.com"), "/") + "/client/v4/accounts/" + accountRef + "/email/sending/send"
		return c.postCloudflare(ctx, endpoint, creds.APIKey, msg)
	case ProviderSES:
		return c.postSES(ctx, accountRef, creds, msg)
	default:
		return ErrPermanent
	}
}

func (c *Client) base(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (c *Client) clock() time.Time {
	if c.Now == nil {
		return time.Now().UTC()
	}
	return c.Now().UTC()
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return Default().HTTP
}

func resendBody(msg Message) any {
	return map[string]any{
		"from":    msg.From,
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"text":    msg.Text,
	}
}

func (c *Client) postJSON(ctx context.Context, endpoint string, apiKey string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ErrPermanent
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ErrPermanent
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return ErrRetry
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ErrRetry
	}
	if err := classify(resp.StatusCode); err != nil {
		return err
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.ID == "" {
		return ErrPermanent
	}
	return nil
}

func (c *Client) postCloudflare(ctx context.Context, endpoint string, apiKey string, msg Message) error {
	raw, err := json.Marshal(map[string]string{
		"to":      msg.To,
		"from":    msg.From,
		"subject": msg.Subject,
		"text":    msg.Text,
	})
	if err != nil {
		return ErrPermanent
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ErrPermanent
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return ErrRetry
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ErrRetry
	}
	if err := classify(resp.StatusCode); err != nil {
		return err
	}
	var parsed struct {
		Success bool `json:"success"`
		Result  struct {
			Delivered []string `json:"delivered"`
			Queued    []string `json:"queued"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || !parsed.Success {
		return ErrPermanent
	}
	if len(parsed.Result.Delivered)+len(parsed.Result.Queued) == 0 {
		return ErrPermanent
	}
	return nil
}

func classify(code int) error {
	if code == http.StatusTooManyRequests || code >= 500 {
		return ErrRetry
	}
	if code >= 400 {
		return ErrPermanent
	}
	if code < 200 || code >= 300 {
		return ErrPermanent
	}
	return nil
}

type secretDocument struct {
	APIKey          string `json:"api_key,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
}

func EncodeSecret(creds Credentials) ([]byte, error) {
	return json.Marshal(secretDocument{
		APIKey:          creds.APIKey,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
	})
}

func ParseSecret(raw []byte) (Credentials, error) {
	var wire secretDocument
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Credentials{}, err
	}
	return Credentials{
		APIKey:          wire.APIKey,
		AccessKeyID:     wire.AccessKeyID,
		SecretAccessKey: wire.SecretAccessKey,
	}, nil
}

func Fits(provider string, creds Credentials) bool {
	switch provider {
	case ProviderResend, ProviderMillionSend, ProviderCloudflare:
		return creds.APIKey != "" && creds.AccessKeyID == "" && creds.SecretAccessKey == ""
	case ProviderSES:
		return creds.APIKey == "" && creds.AccessKeyID != "" && creds.SecretAccessKey != ""
	default:
		return false
	}
}
