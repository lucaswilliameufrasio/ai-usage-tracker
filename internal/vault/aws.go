package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ai-usage-tracker/internal/awsig"
)

// SecretsManager uses the process credential chain's resolved keys.
// The UI never receives an access key. KMS alone is not this port.
type SecretsManager struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Prefix          string
	Endpoint        string
	HTTP            *http.Client
	Now             func() time.Time
}

func (a *SecretsManager) Put(ctx context.Context, name string, plaintext []byte) error {
	if err := checkName(name); err != nil {
		return err
	}
	if err := checkPlain(plaintext); err != nil {
		return err
	}
	id := a.secretID(name)
	status, body, err := a.invoke(ctx, "secretsmanager.CreateSecret", map[string]any{
		"Name":         id,
		"SecretString": string(plaintext),
	})
	if err != nil {
		return err
	}
	if statusErr(status) == nil {
		return nil
	}
	if !awsType(body, "ResourceExistsException") {
		return awsStatus(status, body)
	}
	status, body, err = a.invoke(ctx, "secretsmanager.PutSecretValue", map[string]any{
		"SecretId":     id,
		"SecretString": string(plaintext),
	})
	if err != nil {
		return err
	}
	return awsStatus(status, body)
}

func (a *SecretsManager) Get(ctx context.Context, name string) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	status, body, err := a.invoke(ctx, "secretsmanager.GetSecretValue", map[string]any{
		"SecretId": a.secretID(name),
	})
	if err != nil {
		return nil, err
	}
	if err := awsStatus(status, body); err != nil {
		return nil, err
	}
	var parsed struct {
		SecretString string `json:"SecretString"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.SecretString == "" {
		return nil, ErrSealed
	}
	if len(parsed.SecretString) > maxPlain {
		return nil, ErrTooLarge
	}
	return []byte(parsed.SecretString), nil
}

func (a *SecretsManager) Delete(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	status, body, err := a.invoke(ctx, "secretsmanager.DeleteSecret", map[string]any{
		"SecretId":                   a.secretID(name),
		"ForceDeleteWithoutRecovery": true,
	})
	if err != nil {
		return err
	}
	if awsType(body, "ResourceNotFoundException") {
		return nil
	}
	return awsStatus(status, body)
}

func (a *SecretsManager) Present(ctx context.Context, name string) (bool, error) {
	_, err := a.Get(ctx, name)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *SecretsManager) invoke(ctx context.Context, target string, payload map[string]any) (int, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	endpoint := a.endpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	req.Host = req.URL.Host
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	amzDate := now.Format("20060102T150405Z")
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Target", target)
	headers := map[string]string{
		"content-type": "application/x-amz-json-1.1",
		"host":         req.Host,
		"x-amz-date":   amzDate,
		"x-amz-target": target,
	}
	if a.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", a.SessionToken)
		headers["x-amz-security-token"] = a.SessionToken
	}
	req.Header.Set("Authorization", awsig.Sign(
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		req.Host,
		amzDate,
		a.Region,
		"secretsmanager",
		a.AccessKeyID,
		a.SecretAccessKey,
		raw,
		headers,
	))
	return do(ctx, a.HTTP, req)
}

func (a *SecretsManager) endpoint() string {
	if strings.TrimSpace(a.Endpoint) != "" {
		return strings.TrimRight(a.Endpoint, "/") + "/"
	}
	return "https://secretsmanager." + a.Region + ".amazonaws.com/"
}

func (a *SecretsManager) secretID(name string) string {
	prefix := strings.Trim(a.Prefix, "/")
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func awsStatus(code int, body []byte) error {
	if awsType(body, "ResourceNotFoundException") {
		return ErrNotFound
	}
	if awsType(body, "AccessDeniedException") || awsType(body, "UnrecognizedClientException") {
		return ErrSealed
	}
	return statusErr(code)
}

func awsType(body []byte, name string) bool {
	var parsed struct {
		Type string `json:"__type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	return strings.Contains(parsed.Type, name)
}
