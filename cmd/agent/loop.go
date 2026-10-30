package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-usage-tracker/internal/agent"
)

const (
	repeatEvery = 5 * time.Minute
	passBudget  = 40 * time.Second
)

var errToken = errors.New("token ilegível")

var applyUpdate = agent.ApplyUpdate

func executeLoop(file agent.File) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintln(os.Stderr, "o agente repete a cada 5 minutos; Ctrl+C espera o envio atual e mantém a fila")
	os.Exit(runPasses(ctx, repeatEvery, passBudget, passWithUpdate(ctx, file, func(sendCtx context.Context) (agent.Result, error) {
		return runFile(sendCtx, file)
	}), waitInterruptible))
}

func passWithUpdate(signalCtx context.Context, file agent.File, send func(context.Context) (agent.Result, error)) func(context.Context) (agent.Result, error) {
	return func(sendCtx context.Context) (agent.Result, error) {
		if file.AutoUpdate {
			updateCtx, cancel := context.WithTimeout(signalCtx, 30*time.Second)
			err := applyUpdate(updateCtx, agent.UpdateOptions{CurrentVersion: agent.Version})
			cancel()
			if signalCtx.Err() != nil {
				return agent.Result{}, nil
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "atualização ignorada; o envio segue")
			}
		}
		return send(sendCtx)
	}
}

func runPasses(ctx context.Context, interval time.Duration, budget time.Duration, pass func(context.Context) (agent.Result, error), wait func(context.Context, time.Duration) bool) int {
	for {
		if ctx.Err() != nil {
			return 0
		}
		passCtx, cancel := context.WithTimeout(context.Background(), budget)
		result, err := pass(passCtx)
		cancel()
		code := report(result, err)
		if result.Blocked || errors.Is(err, errToken) {
			return code
		}
		if ctx.Err() != nil {
			if err != nil {
				return 1
			}
			return 0
		}
		if !wait(ctx, interval) {
			return 0
		}
	}
}

func waitInterruptible(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
