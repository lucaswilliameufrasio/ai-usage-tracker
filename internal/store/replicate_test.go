package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
	"github.com/lucaswilliameufrasio/sqlite-replication"
)

func TestReplicationRestoresATemporaryDatabase(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "data", "tracker.db")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := refuseLiveDatabase(source); err != nil {
		t.Fatal(err)
	}
	db, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	account, err := db.UpsertAccount("github", "42", "pessoa@exemplo.test", "acct_um", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateEnrollment("code-a", account.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.Enroll("code-a", "dvc_pessoa", "caderno", "hash-a", now); err != nil {
		t.Fatal(err)
	}
	sample := contract.Sample{
		SampleID:    "pessoa-aaaa",
		Provider:    contract.ProviderClaude,
		CollectedAt: now,
		Windows: []contract.Window{{
			Name: "five_hour", Used: 10, Limit: 100, Unit: "percent", ResetsAt: now.Add(5 * time.Hour),
		}},
	}
	if _, err := db.InsertSample("dvc_pessoa", sample, now); err != nil {
		t.Fatal(err)
	}
	second := sample
	second.SampleID = "pessoa-bbbb"
	second.CollectedAt = now.Add(time.Minute)
	if _, err := db.InsertSample("dvc_pessoa", second, now); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal %s", mode)
	}
	before, err := db.Census()
	if err != nil {
		t.Fatal(err)
	}
	if before.UserVersion != schemaVersion || before.Accounts != 1 || before.Devices != 1 || before.Samples != 2 {
		t.Fatalf("censo inicial %+v", before)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	repository := filepath.Join(dir, "replica")
	if _, err := replication.Snapshot(ctx, source, repository); err != nil {
		t.Fatal(err)
	}
	restoredPath := filepath.Join(dir, "restored.sqlite")
	if err := replication.RestoreSnapshot(ctx, repository, restoredPath); err != nil {
		t.Fatal(err)
	}
	frozen, err := Inspect(restoredPath)
	if err != nil || frozen != before {
		t.Fatalf("restauração %+v %v", frozen, err)
	}
	opened, err := Open(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	records, err := opened.ListSamples("", time.Time{}, "", 10)
	if err != nil || len(records) != 2 {
		t.Fatalf("amostras restauradas %d %v", len(records), err)
	}

	live, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = live.Close() })
	third := sample
	third.SampleID = "pessoa-cccc"
	third.CollectedAt = now.Add(2 * time.Minute)
	if _, err := live.InsertSample("dvc_pessoa", third, now); err != nil {
		t.Fatal(err)
	}
	after, err := live.Census()
	if err != nil || after.Samples != 3 {
		t.Fatalf("origem depois %+v %v", after, err)
	}
	again, err := Inspect(restoredPath)
	if err != nil || again != before {
		t.Fatalf("restauração mudou %+v %v", again, err)
	}
	if _, err := replication.Snapshot(ctx, source, repository); err == nil || !strings.Contains(err.Error(), "immutable key") {
		t.Fatal(err)
	}
}

func TestReplicationRejectsDeleteJournal(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "delete.db")
	if err := refuseLiveDatabase(source); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+source+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE marker (id INTEGER PRIMARY KEY)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(mode, "wal") {
		t.Fatalf("journal %s", mode)
	}
	if _, err := replication.Snapshot(context.Background(), source, filepath.Join(dir, "replica")); err == nil {
		t.Fatal("snapshot aceitou journal delete")
	}
}

func refuseLiveDatabase(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	live, err := filepath.Abs(filepath.Join("..", "..", "data", "tracker.db"))
	if err != nil {
		return err
	}
	if abs == live {
		return fmt.Errorf("ensaio apontou para o banco em uso")
	}
	return nil
}
