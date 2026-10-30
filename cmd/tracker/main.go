package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ai-usage-tracker/internal/httpapi"
	"ai-usage-tracker/internal/push"
	"ai-usage-tracker/internal/store"
)

func main() {
	addr := flag.String("addr", env("TRACKER_ADDR", "127.0.0.1:8787"), "endereço de escuta")
	data := flag.String("data", env("TRACKER_DATA_DIR", "data"), "diretório do SQLite e das chaves locais")
	web := flag.String("web", env("TRACKER_WEB_DIR", ""), "diretório do PWA compilado")
	flag.Parse()
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := os.MkdirAll(*data, 0o700); err != nil {
		logger.Error("dados", "err", "diretório")
		os.Exit(1)
	}
	db, err := store.Open(filepath.Join(*data, "tracker.db"))
	if err != nil {
		logger.Error("sqlite")
		os.Exit(1)
	}
	defer db.Close()
	sender, err := push.LoadOrCreate(filepath.Join(*data, "vapid.json"))
	if err != nil {
		logger.Error("vapid")
		os.Exit(1)
	}
	server := &httpapi.Server{
		Store:          db,
		Sender:         sender,
		AllowOpenSetup: loopback(*addr),
		SetupToken:     os.Getenv("TRACKER_SETUP_TOKEN"),
		WebDir:         *web,
		Logger:         logger,
	}
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	logger.Info("escuta", "addr", *addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("servidor")
		os.Exit(1)
	}
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
