package store

import (
	"path/filepath"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
)

func TestAccountKeepsContactAndOwnsDevices(t *testing.T) {
	db := openDB(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	first, err := db.UpsertAccount("github", "42", "pessoa@exemplo.test", "acct_um", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.UpsertAccount("github", "42", "", "acct_outro", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Contact != "pessoa@exemplo.test" {
		t.Fatal("contato apagado")
	}
	third, err := db.UpsertAccount("github", "42", "outro@exemplo.test", "acct_terceiro", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if third.ID != first.ID || third.Contact != "outro@exemplo.test" {
		t.Fatal("contato novo")
	}
	if err := db.CreateEnrollment("code-a", first.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateEnrollment("code-b", "", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.Enroll("code-a", "dvc_pessoa", "caderno", "hash-a", now); err != nil {
		t.Fatal(err)
	}
	if err := db.Enroll("code-b", "dvc_instalacao", "mesa", "hash-b", now); err != nil {
		t.Fatal(err)
	}
	owned := contract.Sample{
		SampleID:    "pessoa-aaaa",
		Provider:    contract.ProviderClaude,
		CollectedAt: now,
		Windows: []contract.Window{{
			Name: "five_hour", Used: 10, Limit: 100, Unit: "percent", ResetsAt: now.Add(5 * time.Hour),
		}},
	}
	other := owned
	other.SampleID = "outra-bbbb"
	other.CollectedAt = now.Add(-time.Hour)
	if _, err := db.InsertSample("dvc_pessoa", owned, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertSample("dvc_instalacao", other, now); err != nil {
		t.Fatal(err)
	}
	records, err := db.ListSamplesFor(first.ID, "", time.Time{}, "", 10)
	if err != nil || len(records) != 1 || records[0].Sample.SampleID != "pessoa-aaaa" {
		t.Fatalf("amostras da conta %d", len(records))
	}
	all, err := db.ListSamples("", time.Time{}, "", 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("instalação %d", len(all))
	}
	devices, err := db.ListDevicesFor(first.ID)
	if err != nil || len(devices) != 1 || devices[0].ID != "dvc_pessoa" {
		t.Fatal("dispositivos")
	}
	if _, err := db.RevokeOwned("", "dvc_pessoa", now); !errorsIsNotFound(err) {
		t.Fatal("revogação alheia")
	}
	if _, err := db.RevokeOwned(first.ID, "dvc_instalacao", now); !errorsIsNotFound(err) {
		t.Fatal("revogação cruzada")
	}
	revoked, err := db.RevokeOwned(first.ID, "dvc_pessoa", now)
	if err != nil || revoked.RevokedAt == nil {
		t.Fatal("revogação própria")
	}
}

func TestAttemptIsSingleUse(t *testing.T) {
	db := openDB(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	attempt := LoginAttempt{
		ID:           "att_um",
		Provider:     "github",
		Nonce:        "non_um",
		CodeVerifier: "pkc_um",
		ClientID:     "id-pub",
		SecretRef:    "login.attempt.abcdef0123456789",
		RedirectURI:  "http://exemplo.test/v1/login/github/callback",
		ExpiresAt:    now.Add(10 * time.Minute),
	}
	if err := db.InsertAttempt(attempt, "hash-estado"); err != nil {
		t.Fatal(err)
	}
	got, err := db.TakeAttempt("hash-estado", now)
	if err != nil || got.SecretRef != attempt.SecretRef || !got.Used {
		t.Fatal("primeira troca")
	}
	if _, err := db.TakeAttempt("hash-estado", now); err != ErrLoginState {
		t.Fatal("reuso")
	}
	expired := attempt
	expired.ID = "att_velho"
	expired.ExpiresAt = now.Add(-time.Minute)
	if err := db.InsertAttempt(expired, "hash-velho"); err != nil {
		t.Fatal(err)
	}
	stale, err := db.TakeAttempt("hash-velho", now)
	if err != ErrLoginExpired || stale.SecretRef == "" {
		t.Fatal("expirado")
	}
	if _, err := db.TakeAttempt("ausente", now); err != ErrLoginState {
		t.Fatal("ausente")
	}
}

func openDB(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func errorsIsNotFound(err error) bool {
	return err == ErrNotFound
}
