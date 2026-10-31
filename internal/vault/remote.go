package vault

import (
	"context"
	"io"
	"net/http"
	"time"
)

const maxResponse = 1 << 20

func checkName(name string) error {
	if !namePattern.MatchString(name) {
		return ErrInvalidName
	}
	return nil
}

func checkPlain(plaintext []byte) error {
	if len(plaintext) == 0 {
		return ErrInvalidName
	}
	if len(plaintext) > maxPlain {
		return ErrTooLarge
	}
	return nil
}

func httpClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func do(ctx context.Context, client *http.Client, req *http.Request) (int, []byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, ErrUnavailable
	}
	resp, err := httpClient(client).Do(req)
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return resp.StatusCode, nil, ErrUnavailable
	}
	if len(body) > maxResponse {
		return resp.StatusCode, nil, ErrTooLarge
	}
	return resp.StatusCode, body, nil
}

func statusErr(code int) error {
	if code >= 200 && code < 300 {
		return nil
	}
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrSealed
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return ErrUnavailable
	}
}
