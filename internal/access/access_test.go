package access

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

func TestSeedDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	db, box, dbPath := openPair(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	env := map[string]string{
		"TRACKER_LOGIN_GITHUB_CLIENT_ID":     "id-um",
		"TRACKER_LOGIN_GITHUB_CLIENT_SECRET": "segredo-um-sintetico",
	}
	if err := Seed(ctx, db, box, env, now); err != nil {
		t.Fatal(err)
	}
	env["TRACKER_LOGIN_GITHUB_CLIENT_SECRET"] = "segredo-dois-sintetico"
	env["TRACKER_LOGIN_GITHUB_CLIENT_ID"] = "id-dois"
	if err := Seed(ctx, db, box, env, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := box.Get(ctx, SecretRef(GitHub))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "segredo-um-sintetico" {
		t.Fatal("ambiente sobrescreveu o segredo")
	}
	rows, err := db.ListLoginProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ClientID != "id-um" {
		t.Fatalf("config %#v", rows)
	}
	catalog, err := Catalog(ctx, db, box)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range catalog {
		if item.Provider == GitHub && (!item.Configured || !item.SecretPresent || !item.Enabled) {
			t.Fatalf("github %#v", item)
		}
	}
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("segredo-um-sintetico")) {
		t.Fatal("sqlite guarda o segredo")
	}
}

func TestSaveRefusesLastMethod(t *testing.T) {
	ctx := context.Background()
	db, box, _ := openPair(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	secret := []byte("segredo-github-sintetico")
	err := Save(ctx, db, box, Draft{
		Provider: GitHub,
		Enabled:  true,
		ClientID: "id",
		Secret:   &secret,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = Save(ctx, db, box, Draft{
		Provider: GitHub,
		Enabled:  false,
		ClientID: "id",
		Now:      now,
	})
	if err != store.ErrLastLogin {
		t.Fatalf("último: %v", err)
	}
}

func TestRejectedDisableDoesNotRotateSecret(t *testing.T) {
	ctx := context.Background()
	db, box, _ := openPair(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	first := []byte("segredo-um-sintetico")
	err := Save(ctx, db, box, Draft{
		Provider: GitHub,
		Enabled:  true,
		ClientID: "id",
		Secret:   &first,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	second := []byte("segredo-dois-sintetico")
	err = Save(ctx, db, box, Draft{
		Provider: GitHub,
		Enabled:  false,
		ClientID: "id",
		Secret:   &second,
		Now:      now,
	})
	if err != store.ErrLastLogin {
		t.Fatalf("último: %v", err)
	}
	got, err := box.Get(ctx, SecretRef(GitHub))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(first) {
		t.Fatal("recusa rotacionou o segredo")
	}
}

func TestAppleRejectsRawSecret(t *testing.T) {
	ctx := context.Background()
	db, box, _ := openPair(t)
	secret := []byte("nao-e-chave")
	err := Save(ctx, db, box, Draft{
		Provider:   Apple,
		Enabled:    true,
		TeamID:     "TEAM",
		KeyID:      "KEY",
		ServicesID: "app.example",
		Secret:     &secret,
		Now:        time.Now().UTC(),
	})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("apple: %v", err)
	}
	_, err = box.Get(ctx, SecretRef(Apple))
	if !errors.Is(err, vault.ErrNotFound) {
		t.Fatal("chave recusada foi gravada")
	}
}

func TestIncompleteEnv(t *testing.T) {
	db, box, _ := openPair(t)
	err := Seed(context.Background(), db, box, map[string]string{
		"TRACKER_LOGIN_GOOGLE_CLIENT_ID": "só-id",
	}, time.Now())
	if err != ErrIncomplete {
		t.Fatalf("incompleto: %v", err)
	}
}

func openPair(t *testing.T) (*store.Store, *vault.Own, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracker.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := vault.OpenOwn(filepath.Join(dir, "vault"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return db, box, dbPath
}
