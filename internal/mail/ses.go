package mail

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
)

func (c *Client) postSES(ctx context.Context, region string, creds Credentials, msg Message) error {
	raw, err := json.Marshal(map[string]any{
		"FromEmailAddress": msg.From,
		"Destination": map[string]any{
			"ToAddresses": []string{msg.To},
		},
		"Content": map[string]any{
			"Simple": map[string]any{
				"Subject": map[string]string{"Data": msg.Subject, "Charset": "UTF-8"},
				"Body": map[string]any{
					"Text": map[string]string{"Data": msg.Text, "Charset": "UTF-8"},
				},
			},
		},
	})
	if err != nil {
		return ErrPermanent
	}
	base := strings.TrimRight(c.SESBase, "/")
	if base == "" {
		base = "https://email." + region + ".amazonaws.com"
	}
	endpoint := base + "/v2/email/outbound-emails"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ErrPermanent
	}
	req.Host = req.URL.Host
	amzDate := c.clock().Format("20060102T150405Z")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amz-Date", amzDate)
	authorization := signV4(req.Method, req.URL.EscapedPath(), req.URL.RawQuery, req.Host, amzDate, region, "ses", creds.AccessKeyID, creds.SecretAccessKey, raw, map[string]string{
		"content-type": "application/json",
		"host":         req.Host,
		"x-amz-date":   amzDate,
	})
	req.Header.Set("Authorization", authorization)
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
		MessageID string `json:"MessageId"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.MessageID == "" {
		return ErrPermanent
	}
	return nil
}

func signV4(method string, path string, query string, host string, amzDate string, region string, service string, accessKey string, secret string, payload []byte, headers map[string]string) string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.TrimSpace(headers[name]))
		canonicalHeaders.WriteByte('\n')
	}
	signed := strings.Join(names, ";")
	sum := sha256.Sum256(payload)
	canonical := strings.Join([]string{
		method,
		path,
		query,
		canonicalHeaders.String(),
		signed,
		hex.EncodeToString(sum[:]),
	}, "\n")
	date := amzDate[:8]
	scope := date + "/" + region + "/" + service + "/aws4_request"
	requestHash := sha256.Sum256([]byte(canonical))
	toSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(requestHash[:]),
	}, "\n")
	key := hmacSHA256([]byte("AWS4"+secret), date)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, toSign))
	return "AWS4-HMAC-SHA256 Credential=" + accessKey + "/" + scope + ", SignedHeaders=" + signed + ", Signature=" + signature
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
