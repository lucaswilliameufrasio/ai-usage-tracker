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
CREATE TABLE push_deliveries (
  id TEXT PRIMARY KEY,
  idempotency_key TEXT NOT NULL UNIQUE,
  subscription_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  provider TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL,
  detail TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
INSERT INTO operator(id, token_hash, created_at) VALUES (1, 'hash-mantido', '2026-10-29T00:00:00Z');
INSERT INTO devices(id, name, token_hash, created_at) VALUES ('dvc_antigo', 'antigo', 'hash-device', '2026-10-29T00:00:00Z');
INSERT INTO push_deliveries(id, idempotency_key, subscription_id, kind, provider, title, body, status, detail, created_at, updated_at)
VALUES ('dly_antigo', 'chave-antiga', '', 'threshold', 'claude', 't', 'b', 'sent', '', '2026-10-29T00:00:00Z', '2026-10-29T00:00:00Z');
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
	if version != schemaVersion {
		t.Fatalf("versão %d", version)
	}
	var attemptsColumn int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('push_deliveries') WHERE name = 'attempts'`).Scan(&attemptsColumn); err != nil {
		t.Fatal(err)
	}
	if attemptsColumn != 1 {
		t.Fatal("fila sem tentativas")
	}
	deliveries, err := db.ListDeliveries(5)
	if err != nil || len(deliveries) != 1 || deliveries[0].ID != "dly_antigo" || deliveries[0].Status != "sent" || deliveries[0].Attempts != 0 || !deliveries[0].NextAttempt.IsZero() {
		t.Fatalf("%+v %v", deliveries, err)
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
