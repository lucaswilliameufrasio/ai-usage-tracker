package vault

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnRoundTripHidesPlaintext(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{7}, 32)
	box, err := OpenOwn(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	secret := []byte("sintetico-segredo-github")
	if err := box.Put(ctx, "login.github", secret); err != nil {
		t.Fatal(err)
	}
	got, err := box.Get(ctx, "login.github")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("leitura diferente")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("arquivos %d", len(entries))
	}
	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, secret) {
		t.Fatal("arquivo contém o segredo")
	}
	if entries[0].Name() == "login.github" {
		t.Fatal("nome do segredo virou arquivo")
	}
}

func TestOwnRejectsWrongKeyAndTamper(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	box, err := OpenOwn(dir, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Put(ctx, "login.google", []byte("outro-segredo-sintetico")); err != nil {
		t.Fatal(err)
	}
	other, err := OpenOwn(dir, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Get(ctx, "login.google"); err != ErrSealed {
		t.Fatalf("chave errada: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, entries[0].Name())
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Get(ctx, "login.google"); err != ErrSealed {
		t.Fatalf("arquivo alterado: %v", err)
	}
}

func TestOwnRejectsPathAndMissing(t *testing.T) {
	dir := t.TempDir()
	box, err := OpenOwn(dir, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := box.Put(ctx, "../login.github", []byte("x")); err != ErrInvalidName {
		t.Fatalf("nome: %v", err)
	}
	if _, err := box.Get(ctx, "login.github"); err != ErrNotFound {
		t.Fatalf("ausente: %v", err)
	}
	if err := box.Delete(ctx, "login.github"); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeKey("curta"); err != ErrInvalidKey {
		t.Fatalf("chave: %v", err)
	}
}
