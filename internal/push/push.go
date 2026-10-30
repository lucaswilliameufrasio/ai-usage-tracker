package push

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrGone      = errors.New("subscription gone")
	ErrTemporary = errors.New("push temporary")
	ErrRejected  = errors.New("push rejected")
)

type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

type Sender interface {
	Configured() bool
	PublicKey() string
	Send(ctx context.Context, sub Subscription, payload []byte) error
}

type Recording struct {
	mu        sync.Mutex
	payloads  [][]byte
	gone      map[string]bool
	key       string
	temporary int
	rejected  bool
}

func NewRecording(publicKey string) *Recording {
	return &Recording{key: publicKey, gone: map[string]bool{}}
}

func (r *Recording) Configured() bool {
	return r.key != ""
}

func (r *Recording) PublicKey() string {
	return r.key
}

func (r *Recording) Send(_ context.Context, sub Subscription, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gone[sub.Endpoint] {
		return ErrGone
	}
	if r.rejected {
		return ErrRejected
	}
	if r.temporary > 0 {
		r.temporary--
		return ErrTemporary
	}
	copied := append([]byte(nil), payload...)
	r.payloads = append(r.payloads, copied)
	return nil
}

func (r *Recording) Payloads() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.payloads))
	copy(out, r.payloads)
	return out
}

func (r *Recording) FailEndpoint(endpoint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gone[endpoint] = true
}

func (r *Recording) FailTemporary(times int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.temporary = times
}

func (r *Recording) FailRejected() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejected = true
}
