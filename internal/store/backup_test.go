package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
)

func TestSnapshotRestoresCountsAndLeavesTheSource(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "data", "tracker.db")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	before, err := db.Census()
	if err != nil {
		t.Fatal(err)
	}
	if before.UserVersion != 3 || before.Accounts != 1 || before.Devices != 1 || before.Samples != 2 {
		t.Fatalf("censo inicial %+v", before)
	}
	snapshot := filepath.Join(dir, "snapshots", "snapshot.db")
	if err := os.MkdirAll(filepath.Dir(snapshot), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SameTarget(sourcePath, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := SameTarget(sourcePath, sourcePath); !errors.Is(err, ErrSnapshotSame) {
		t.Fatal(err)
	}
	written, err := db.Snapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if written != before {
		t.Fatalf("snapshot %+v origem %+v", written, before)
	}
	info, err := os.Stat(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("modo %o", info.Mode().Perm())
	}
	if _, err := db.Snapshot(snapshot); !errors.Is(err, ErrSnapshotExists) {
		t.Fatal(err)
	}
	third := sample
	third.SampleID = "pessoa-cccc"
	third.CollectedAt = now.Add(2 * time.Minute)
	if _, err := db.InsertSample("dvc_pessoa", third, now); err != nil {
		t.Fatal(err)
	}
	after, err := db.Census()
	if err != nil || after.Samples != 3 {
		t.Fatalf("origem depois %+v %v", after, err)
	}
	frozen, err := Inspect(snapshot)
	if err != nil || frozen != before {
		t.Fatalf("snapshot mudou %+v %v", frozen, err)
	}
	restoredDir := filepath.Join(dir, "restored")
	restored, err := Restore(snapshot, restoredDir)
	if err != nil || restored != before {
		t.Fatalf("restauração %+v %v", restored, err)
	}
	opened, err := Open(filepath.Join(restoredDir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	records, err := opened.ListSamples("", time.Time{}, "", 10)
	if err != nil || len(records) != 2 {
		t.Fatalf("amostras restauradas %d %v", len(records), err)
	}
	marker := filepath.Join(restoredDir, "tracker.db")
	kept, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(snapshot, restoredDir); !errors.Is(err, ErrRestoreExists) {
		t.Fatal(err)
	}
	again, err := os.ReadFile(marker)
	if err != nil || string(again) != string(kept) {
		t.Fatal("restauração alterou o banco existente")
	}
	live, err := db.Census()
	if err != nil || live.Samples != 3 {
		t.Fatalf("origem viva %+v %v", live, err)
	}
}

func TestRestoreRejectsACorruptSnapshot(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(bad, []byte("nao-e-sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "dest")
	if _, err := Restore(bad, dest); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "tracker.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
