package push

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyPushStatus(t *testing.T) {
	if err := classifyPushStatus(http.StatusNoContent); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(classifyPushStatus(http.StatusGone), ErrGone) {
		t.Fatal("410")
	}
	if !errors.Is(classifyPushStatus(http.StatusNotFound), ErrGone) {
		t.Fatal("404")
	}
	if !errors.Is(classifyPushStatus(http.StatusTooManyRequests), ErrTemporary) {
		t.Fatal("429")
	}
	if !errors.Is(classifyPushStatus(http.StatusBadGateway), ErrTemporary) {
		t.Fatal("502")
	}
	if !errors.Is(classifyPushStatus(http.StatusBadRequest), ErrRejected) {
		t.Fatal("400")
	}
}
