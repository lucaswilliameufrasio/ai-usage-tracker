package release_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInstallerRejectsBadChecksumAndInstallsAMatchingArchive(t *testing.T) {
	root := repoRoot(t)
	osName, archName := hostNames()
	asset := "ai-usage_0.1.0_" + osName + "_" + archName + ".tar.gz"
	pub := t.TempDir()
	payload := []byte("#!/bin/sh\nprintf 'agente-de-teste\\n'\n")
	writeArchive(t, filepath.Join(pub, asset), payload)
	sum := fileSHA256(t, filepath.Join(pub, asset))
	if err := os.WriteFile(filepath.Join(pub, "checksums.txt"), []byte("0000000000000000000000000000000000000000000000000000000000000000  "+asset+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(pub)))
	t.Cleanup(server.Close)
	binDir := t.TempDir()
	bad := runInstaller(t, root, server.URL, binDir, true)
	if bad.exit == 0 || !strings.Contains(bad.output, "Checksum não confere") {
		t.Fatalf("checksum ruim saiu %d: %s", bad.exit, bad.output)
	}
	if _, err := os.Stat(filepath.Join(binDir, "ai-usage")); !os.IsNotExist(err) {
		t.Fatal("checksum ruim instalou o binário")
	}

	if err := os.WriteFile(filepath.Join(pub, "checksums.txt"), []byte(sum+"  "+asset+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := runInstaller(t, root, server.URL, binDir, true)
	if good.exit != 0 {
		t.Fatalf("instalação %d: %s", good.exit, good.output)
	}
	out, err := exec.Command(filepath.Join(binDir, "ai-usage")).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "agente-de-teste\n" {
		t.Fatalf("binário %q", out)
	}

	again := runInstaller(t, root, server.URL, binDir, false)
	if again.exit == 0 || !strings.Contains(again.output, "Sem terminal") {
		t.Fatalf("reinstalação sem --force saiu %d: %s", again.exit, again.output)
	}
	kept, err := os.ReadFile(filepath.Join(binDir, "ai-usage"))
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != string(payload) {
		t.Fatal("reinstalação recusada trocou o binário")
	}
}

func TestInstallerRejectsAnUnstableTag(t *testing.T) {
	result := runInstallerArgs(t, repoRoot(t), []string{"--tag", "v1", "--base-url", "http://127.0.0.1:9"}, t.TempDir(), false)
	if result.exit == 0 || !strings.Contains(result.output, "Tag de release estável inválida") {
		t.Fatalf("tag %d: %s", result.exit, result.output)
	}
}

func TestInstallerRequiresATagForBaseURL(t *testing.T) {
	result := runInstallerArgs(t, repoRoot(t), []string{"--base-url", "http://127.0.0.1:9"}, t.TempDir(), false)
	if result.exit == 0 || !strings.Contains(result.output, "Informe --tag ao usar --base-url") {
		t.Fatalf("base sem tag saiu %d: %s", result.exit, result.output)
	}
}

func TestPackagedAgentInstalls(t *testing.T) {
	if testing.Short() {
		t.Skip("empacota o agente de verdade")
	}
	root := repoRoot(t)
	out := t.TempDir()
	cmd := exec.Command("sh", filepath.Join(root, "scripts/package-agent.sh"), "--version", "0.1.0", "--out", out, "--target", runtime.GOOS+"/"+runtime.GOARCH)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pacote: %v\n%s", err, output)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(out)))
	t.Cleanup(server.Close)
	binDir := t.TempDir()
	installed := runInstaller(t, root, server.URL, binDir, true)
	if installed.exit != 0 {
		t.Fatalf("instalação %d: %s", installed.exit, installed.output)
	}
	agent := exec.Command(filepath.Join(binDir, "ai-usage"))
	raw, err := agent.CombinedOutput()
	if agent.ProcessState == nil || agent.ProcessState.ExitCode() != 2 {
		t.Fatalf("uso saiu %v: %s", err, raw)
	}
	if !strings.Contains(string(raw), "uso: ai-usage enroll | ai-usage run | ai-usage setup") {
		t.Fatalf("uso %q", raw)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, "..", ".."))
}

func hostNames() (string, string) {
	osName := runtime.GOOS
	archName := runtime.GOARCH
	return osName, archName
}

func writeArchive(t *testing.T, path string, payload []byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	header := &tar.Header{
		Name: "ai-usage",
		Mode: 0o755,
		Size: int64(len(payload)),
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type installerResult struct {
	exit   int
	output string
}

func runInstaller(t *testing.T, root string, baseURL string, binDir string, force bool) installerResult {
	t.Helper()
	args := []string{"--tag", "v0.1.0", "--base-url", baseURL, "--bin-dir", binDir}
	if force {
		args = append(args, "--force")
	}
	return runInstallerArgs(t, root, args, binDir, !force)
}

func runInstallerArgs(t *testing.T, root string, args []string, binDir string, detach bool) installerResult {
	t.Helper()
	script := filepath.Join(root, "scripts/agent-installer.sh")
	commandArgs := []string{script}
	commandArgs = append(commandArgs, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", commandArgs...)
	if detach {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	cmd.Dir = binDir
	raw, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("instalador não terminou: %s", raw)
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatal(err)
		}
		exit = exitErr.ExitCode()
	}
	return installerResult{exit: exit, output: string(raw)}
}
