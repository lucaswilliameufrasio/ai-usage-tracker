package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevBuildDoesNotTouchTheNetwork(t *testing.T) {
	err := ApplyUpdate(context.Background(), UpdateOptions{
		CurrentVersion: "dev",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("dev consultou a rede")
			return nil, os.ErrClosed
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSameVersionDoesNotDownload(t *testing.T) {
	err := ApplyUpdate(context.Background(), UpdateOptions{
		CurrentVersion: "0.1.0",
		Tag:            "v0.1.0",
		BaseURL:        "http://127.0.0.1:9",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("versão igual baixou a release")
			return nil, os.ErrClosed
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateReplacesExecutableAfterChecksum(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	if err := os.WriteFile(binary, []byte("antigo"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("novo-agente")
	asset := "ai-usage_0.2.0_linux_amd64.tar.gz"
	archive := archiveBytes(t, payload)
	sum := sha256.Sum256(archive)
	pub := t.TempDir()
	if err := os.WriteFile(filepath.Join(pub, asset), archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pub, "checksums.txt"), []byte("0000000000000000000000000000000000000000000000000000000000000000  "+asset+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(hostOnly(pub))
	t.Cleanup(server.Close)
	bad := ApplyUpdate(context.Background(), updateOpts(server, binary, "0.1.0", "v0.2.0"))
	if bad == nil || bad.Error() != "checksum mismatch" {
		t.Fatal(bad)
	}
	kept, err := os.ReadFile(binary)
	if err != nil || string(kept) != "antigo" {
		t.Fatal("checksum ruim substituiu o binário")
	}
	if err := os.WriteFile(filepath.Join(pub, "checksums.txt"), []byte(hex.EncodeToString(sum[:])+"  "+asset+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	replaced := false
	opts := updateOpts(server, binary, "0.1.0", "v0.2.0")
	opts.Exec = func(argv0 string, argv []string, _ []string) error {
		replaced = true
		if argv0 != binary || len(argv) != 1 || argv[0] != "agent" {
			t.Fatalf("exec %s %v", argv0, argv)
		}
		return nil
	}
	opts.Args = []string{"agent"}
	if err := ApplyUpdate(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Fatal("não reiniciou")
	}
	got, err := os.ReadFile(binary)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("binário novo")
	}
	info, err := os.Stat(binary)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatal("modo")
	}
}

func TestUpdateReadsTheLatestTagFromTheTestServer(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	if err := os.WriteFile(binary, []byte("antigo"), 0o755); err != nil {
		t.Fatal(err)
	}
	asset := "ai-usage_0.3.0_linux_amd64.tar.gz"
	archive := archiveBytes(t, []byte("tres"))
	sum := sha256.Sum256(archive)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.0"}`))
	})
	mux.HandleFunc("GET /checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"))
	})
	mux.HandleFunc("GET /"+asset, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	called := false
	err := ApplyUpdate(context.Background(), UpdateOptions{
		CurrentVersion: "0.1.0",
		BaseURL:        server.URL,
		LatestURL:      server.URL + "/latest",
		Client:         lockedClient(server),
		GOOS:           "linux",
		GOARCH:         "amd64",
		Executable:     binary,
		Args:           []string{"agent"},
		Env:            []string{},
		Exec: func(string, []string, []string) error {
			called = true
			return nil
		},
	})
	if err != nil || !called {
		t.Fatal(err)
	}
}

func updateOpts(server *httptest.Server, binary string, current string, tag string) UpdateOptions {
	return UpdateOptions{
		CurrentVersion: current,
		BaseURL:        server.URL,
		Tag:            tag,
		Client:         lockedClient(server),
		GOOS:           "linux",
		GOARCH:         "amd64",
		Executable:     binary,
		Args:           []string{"agent"},
		Env:            []string{},
	}
}

func lockedClient(server *httptest.Server) *http.Client {
	host := strings.TrimPrefix(strings.TrimPrefix(server.URL, "https://"), "http://")
	base := server.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != host {
			return nil, os.ErrPermission
		}
		return base.RoundTrip(r)
	})}
}

func hostOnly(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Host, "github.com") {
			http.Error(w, "github", http.StatusBadGateway)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func archiveBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	header := &tar.Header{Name: "ai-usage", Mode: 0o755, Size: int64(len(payload))}
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
	return buf.Bytes()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
