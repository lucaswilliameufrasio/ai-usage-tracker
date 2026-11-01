package main

import (
	"reflect"
	"testing"

	"ai-usage-tracker/internal/agent"
)

func TestOutcomeLinesNameProviderKindAndState(t *testing.T) {
	got := outcomeLines([]agent.Outcome{
		{Kind: "sample", Provider: "claude", Status: "accepted"},
		{Kind: "sample", Provider: "codex", Status: "duplicate"},
		{Kind: "report", Provider: "antigravity", Status: "rejected", Code: "INVALID_REQUEST"},
		{Kind: "sample", Status: "pending"},
	})
	want := []string{
		"claude sample accepted",
		"codex sample duplicate",
		"antigravity report rejected code=INVALID_REQUEST",
		"unknown sample pending",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("linhas %q", got)
	}
}
