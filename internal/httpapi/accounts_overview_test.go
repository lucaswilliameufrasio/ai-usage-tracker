package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/token"
)

func TestOperatorOverviewSeparatesAccounts(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	handler := (&Server{
		Store:          db,
		AllowOpenSetup: true,
		Now:            func() time.Time { return now },
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler()
	operator := setupOperator(t, handler)

	ana, err := db.UpsertAccount("github", "subj_canario_ana", "ana@exemplo.test", "acct_ana_um", now)
	if err != nil {
		t.Fatal(err)
	}
	blank, err := db.UpsertAccount("github", "subj_canario_blank", "   ", "acct_blank_um", now.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bia, err := db.UpsertAccount("github", "subj_canario_bia", "bia@exemplo.test", "acct_bia_um", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertAccount("google", "subj_canario_idle", "fora@exemplo.test", "acct_idle_um", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	enrollOwned(t, db, "code-ana", ana.ID, "dvc_ana_um", now)
	enrollOwned(t, db, "code-blank", blank.ID, "dvc_blank_um", now)
	enrollOwned(t, db, "code-bia", bia.ID, "dvc_bia_um", now)
	enrollOwned(t, db, "code-orphan", "", "dvc_orphan_um", now)
	insertUsed(t, db, "dvc_ana_um", "ana-codex11", 11, now)
	insertUsed(t, db, "dvc_blank_um", "blank-codex7", 7, now)
	insertUsed(t, db, "dvc_bia_um", "bia-codex22", 22, now)
	insertUsed(t, db, "dvc_orphan_um", "orphan-codex33", 33, now)

	overview := callJSON(t, handler, http.MethodGet, "/v1/overview", operator, nil)
	if overview.Code != http.StatusOK {
		t.Fatalf("visão %d", overview.Code)
	}
	if strings.Contains(overview.Body, "subj_canario") || strings.Contains(overview.Body, "fora@exemplo.test") {
		t.Fatal("visão expôs identificador ou conta sem dispositivo")
	}
	var operatorBody map[string]json.RawMessage
	if err := json.Unmarshal([]byte(overview.Body), &operatorBody); err != nil {
		t.Fatal(err)
	}
	if _, ok := operatorBody["providers"]; ok {
		t.Fatal("visão do operador ainda mistura os provedores")
	}
	var accounts []accountGroup
	if err := json.Unmarshal(operatorBody["accounts"], &accounts); err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 4 {
		t.Fatalf("grupos %d", len(accounts))
	}
	if accounts[0].ID != ana.ID || accounts[0].Label != "ana@exemplo.test" || accounts[0].Login != "github" || usedOf(accounts[0], "codex") != 11 {
		t.Fatalf("ana %#v", accounts[0])
	}
	if accounts[1].ID != blank.ID || accounts[1].Label != "GitHub" || usedOf(accounts[1], "codex") != 7 {
		t.Fatalf("sem contato %#v", accounts[1])
	}
	if accounts[2].ID != bia.ID || accounts[2].Label != "bia@exemplo.test" || usedOf(accounts[2], "codex") != 22 {
		t.Fatalf("bia %#v", accounts[2])
	}
	if accounts[3].ID != "" || accounts[3].Label != "Sem conta" || accounts[3].Login != "" || usedOf(accounts[3], "codex") != 33 {
		t.Fatalf("órfão %#v", accounts[3])
	}

	rawSession := "sess_ana_teste"
	if err := db.CreateSession(token.Hash(rawSession), ana.ID, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	person := trip(t, handler, http.MethodGet, "/v1/account/overview", "", &http.Cookie{Name: sessionCookie, Value: rawSession}, nil)
	if person.Code != http.StatusOK {
		t.Fatalf("pessoa %d", person.Code)
	}
	if strings.Contains(person.Body, "bia@exemplo.test") || strings.Contains(person.Body, "Sem conta") || strings.Contains(person.Body, "subj_canario") {
		t.Fatal("pessoa viu outra conta")
	}
	var personBody map[string]json.RawMessage
	if err := json.Unmarshal([]byte(person.Body), &personBody); err != nil {
		t.Fatal(err)
	}
	if _, ok := personBody["accounts"]; ok {
		t.Fatal("visão da pessoa ganhou grupos")
	}
	var providers []providerCardBody
	if err := json.Unmarshal(personBody["providers"], &providers); err != nil {
		t.Fatal(err)
	}
	if usedInCards(providers, "codex") != 11 {
		t.Fatalf("pessoa %#v", providers)
	}
}

func enrollOwned(t *testing.T, db *store.Store, code string, accountID string, deviceID string, now time.Time) {
	t.Helper()
	if err := db.CreateEnrollment(code, accountID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.Enroll(code, deviceID, "caderno", "hash-"+deviceID, now); err != nil {
		t.Fatal(err)
	}
}

func insertUsed(t *testing.T, db *store.Store, deviceID string, sampleID string, used float64, now time.Time) {
	t.Helper()
	sample := contract.Sample{
		SampleID:    sampleID,
		Provider:    contract.ProviderCodex,
		CollectedAt: now,
		Windows: []contract.Window{{
			Name: "five_hour", Used: used, Limit: 100, Unit: "percent", ResetsAt: now.Add(5 * time.Hour),
		}},
	}
	if _, err := db.InsertSample(deviceID, sample, now); err != nil {
		t.Fatal(err)
	}
}

type accountGroup struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	Login     string             `json:"login"`
	Providers []providerCardBody `json:"providers"`
}

type providerCardBody struct {
	Provider   string `json:"provider"`
	LastSample struct {
		Windows []struct {
			Used float64 `json:"used"`
		} `json:"windows"`
	} `json:"last_sample"`
}

func usedOf(group accountGroup, provider string) float64 {
	return usedInCards(group.Providers, provider)
}

func usedInCards(cards []providerCardBody, provider string) float64 {
	for _, card := range cards {
		if card.Provider != provider || len(card.LastSample.Windows) == 0 {
			continue
		}
		return card.LastSample.Windows[0].Used
	}
	return -1
}
