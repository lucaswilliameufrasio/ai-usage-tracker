package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type WebPush struct {
	public  string
	private string
	subject string
}

func LoadOrCreate(path string) (*WebPush, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		var keys struct {
			Public  string `json:"public_key"`
			Private string `json:"private_key"`
		}
		if err := json.Unmarshal(raw, &keys); err != nil {
			return nil, fmt.Errorf("ler chaves vapid: %w", err)
		}
		if keys.Public == "" || keys.Private == "" {
			return nil, fmt.Errorf("chaves vapid incompletas")
		}
		return &WebPush{public: keys.Public, private: keys.Private, subject: "mailto:operator@localhost"}, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("gerar vapid: %w", err)
	}
	encoded, err := json.Marshal(map[string]string{
		"public_key":  public,
		"private_key": private,
	})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return nil, err
	}
	return &WebPush{public: public, private: private, subject: "mailto:operator@localhost"}, nil
}

func (w *WebPush) Configured() bool {
	return w != nil && w.public != "" && w.private != ""
}

func (w *WebPush) PublicKey() string {
	if w == nil {
		return ""
	}
	return w.public
}

func (w *WebPush) Send(ctx context.Context, sub Subscription, payload []byte) error {
	if !w.Configured() {
		return fmt.Errorf("vapid ausente")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			Auth:   sub.Auth,
			P256dh: sub.P256dh,
		},
	}, &webpush.Options{
		Subscriber:      w.subject,
		VAPIDPublicKey:  w.public,
		VAPIDPrivateKey: w.private,
		TTL:             60,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return classifyPushStatus(resp.StatusCode)
}

func classifyPushStatus(code int) error {
	if code == http.StatusNotFound || code == http.StatusGone {
		return ErrGone
	}
	if code == http.StatusTooManyRequests || code >= 500 {
		return ErrTemporary
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("%w: status %d", ErrRejected, code)
	}
	return nil
}
