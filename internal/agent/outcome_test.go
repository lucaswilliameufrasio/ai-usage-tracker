package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"ai-usage-tracker/internal/spool"
)

func item(kind string, provider string) spool.Item {
	body, _ := json.Marshal(map[string]string{"provider": provider})
	return spool.Item{Kind: kind, State: spool.StatePending, Body: body}
}

func outcomeServer(t *testing.T, status map[string]int, code string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Provider string `json:"provider"`
		}
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status[body.Provider])
		if status[body.Provider] >= 400 {
			_ = json.NewEncoder(w).Encode(map[string]string{"error_code": code})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOutcomesNameEachItem(t *testing.T) {
	srv := outcomeServer(t, map[string]int{"claude": 201, "codex": 200, "antigravity": 422}, "INVALID_REQUEST")
	items := []spool.Item{item(spool.KindSample, "claude"), item(spool.KindSample, "codex"), item(spool.KindReport, "antigravity")}
	result, err := send(context.Background(), Config{API: srv.URL, Token: "t", SpoolPath: filepath.Join(t.TempDir(), "spool.json"), Client: srv.Client()}, items)
	if err != nil {
		t.Fatal(err)
	}
	want := []Outcome{
		{Kind: "sample", Provider: "claude", Status: "accepted"},
		{Kind: "sample", Provider: "codex", Status: "duplicate"},
		{Kind: "report", Provider: "antigravity", Status: "rejected", Code: "INVALID_REQUEST"},
	}
	if !reflect.DeepEqual(result.Outcomes, want) {
		t.Fatalf("resultado %+v", result.Outcomes)
	}
}

func TestOutcomesKeepTheRestPendingAfterAFailure(t *testing.T) {
	srv := outcomeServer(t, map[string]int{"claude": 201, "codex": 500, "antigravity": 201}, "INTERNAL")
	items := []spool.Item{item(spool.KindSample, "claude"), item(spool.KindSample, "codex"), item(spool.KindSample, "antigravity")}
	result, err := send(context.Background(), Config{API: srv.URL, Token: "t", SpoolPath: filepath.Join(t.TempDir(), "spool.json"), Client: srv.Client()}, items)
	if err == nil {
		t.Fatal("falha esperada")
	}
	want := []Outcome{
		{Kind: "sample", Provider: "claude", Status: "accepted"},
		{Kind: "sample", Provider: "codex", Status: "pending"},
		{Kind: "sample", Provider: "antigravity", Status: "pending"},
	}
	if !reflect.DeepEqual(result.Outcomes, want) {
		t.Fatalf("resultado %+v", result.Outcomes)
	}
}
