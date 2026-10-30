package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"ai-usage-tracker/internal/agent"
)

func TestRunPassesLetsTheCurrentSendFinish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	finished := false
	code := runPasses(ctx, time.Hour, time.Second, func(passCtx context.Context) (agent.Result, error) {
		cancel()
		if passCtx.Err() != nil {
			t.Fatal("o envio atual foi cancelado junto com o sinal")
		}
		finished = true
		return agent.Result{Accepted: 1}, nil
	}, func(context.Context, time.Duration) bool {
		t.Fatal("não espera outra passagem")
		return false
	})
	if !finished || code != 0 {
		t.Fatalf("finished %v code %d", finished, code)
	}
}

func TestRunPassesRetriesUntilThePauseIsInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	code := runPasses(ctx, time.Hour, time.Second, func(context.Context) (agent.Result, error) {
		calls++
		return agent.Result{Pending: 1}, errors.New("rede")
	}, func(context.Context, time.Duration) bool {
		cancel()
		return false
	})
	if calls != 1 || code != 0 {
		t.Fatalf("calls %d code %d", calls, code)
	}
}

func TestRunPassesStopsWhenTheSignalArrivesDuringTheSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	code := runPasses(ctx, time.Millisecond, time.Second, func(context.Context) (agent.Result, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return agent.Result{}, errors.New("rede")
	}, waitInterruptible)
	if calls != 2 || code != 1 {
		t.Fatalf("calls %d code %d", calls, code)
	}
}

func TestRunPassesDoesNotRetryARevokedDevice(t *testing.T) {
	calls := 0
	code := runPasses(context.Background(), time.Hour, time.Second, func(context.Context) (agent.Result, error) {
		calls++
		return agent.Result{Blocked: true}, nil
	}, func(context.Context, time.Duration) bool {
		t.Fatal("não espera")
		return false
	})
	if calls != 1 || code != 3 {
		t.Fatalf("calls %d code %d", calls, code)
	}
}

func TestRunPassesCancelsAStuckSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sawCancel := false
	code := runPasses(ctx, time.Hour, 20*time.Millisecond, func(passCtx context.Context) (agent.Result, error) {
		<-passCtx.Done()
		sawCancel = true
		return agent.Result{Pending: 1}, errors.New("prazo")
	}, func(context.Context, time.Duration) bool {
		cancel()
		return false
	})
	if !sawCancel || code != 0 {
		t.Fatalf("cancel %v code %d", sawCancel, code)
	}
}

func TestRunPassesStopsOnAnUnreadableToken(t *testing.T) {
	calls := 0
	code := runPasses(context.Background(), time.Hour, time.Second, func(context.Context) (agent.Result, error) {
		calls++
		return agent.Result{}, errToken
	}, func(context.Context, time.Duration) bool {
		t.Fatal("não espera")
		return false
	})
	if calls != 1 || code != 1 {
		t.Fatalf("calls %d code %d", calls, code)
	}
}
