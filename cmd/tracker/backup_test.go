package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ai-usage-tracker/internal/store"
)

func TestBackupRefusesToReplaceTheLiveDatabase(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(data, "tracker.db")
	db, err := store.Open(live)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	err = runBackup([]string{"--data", data, "--out", live}, ioDiscard(t))
	if !errors.Is(err, store.ErrSnapshotSame) {
		t.Fatal(err)
	}
	after, err := os.ReadFile(live)
	if err != nil || string(after) != string(before) {
		t.Fatal("o banco em uso mudou")
	}
}

func TestRestoreRefusesAnExistingDatabase(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	snapshot := filepath.Join(dir, "snapshot.db")
	if err := runBackup([]string{"--data", data, "--out", snapshot}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() == "" || bytes.Contains(output.Bytes(), []byte("hash")) {
		t.Fatalf("saída inesperada")
	}
	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dest, "tracker.db")
	if err := os.WriteFile(target, []byte("vivo"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runRestore([]string{"--snapshot", snapshot, "--dest", dest}, ioDiscard(t))
	if !errors.Is(err, store.ErrRestoreExists) {
		t.Fatal(err)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "vivo" {
		t.Fatal("o banco existente foi substituído")
	}
}

func ioDiscard(t *testing.T) *bytes.Buffer {
	t.Helper()
	return &bytes.Buffer{}
}
