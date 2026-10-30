package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrateV1KeepsOperator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracker.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
CREATE TABLE operator (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  token_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE enrollment_codes (
  code_hash TEXT PRIMARY KEY,
  expires_at TEXT NOT NULL,
  used_at TEXT
);
CREATE TABLE devices (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  revoked_at TEXT
);
INSERT INTO operator(id, token_hash, created_at) VALUES (1, 'hash-mantido', '2026-10-29T00:00:00Z');
INSERT INTO devices(id, name, token_hash, created_at) VALUES ('dvc_antigo', 'antigo', 'hash-device', '2026-10-29T00:00:00Z');
PRAGMA user_version = 1;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	hash, err := db.OperatorHash()
	if err != nil {
		t.Fatal(err)
	}
	if hash != "hash-mantido" {
		t.Fatalf("hash %s", hash)
	}
	kind, err := db.SecretBackend()
	if err != nil {
		t.Fatal(err)
	}
	if kind != "own" {
		t.Fatalf("backend %s", kind)
	}
	var version int
	if err := db.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("versão %d", version)
	}
	rows, err := db.ListLoginProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("logins %d", len(rows))
	}
	var accountID string
	if err := db.db.QueryRow(`SELECT account_id FROM devices WHERE id = 'dvc_antigo'`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if accountID != "" {
		t.Fatal("dispositivo antigo ganhou conta")
	}
}
