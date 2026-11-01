package main

import (
	"reflect"
	"strings"
	"testing"

	"ai-usage-tracker/internal/agent"
	"ai-usage-tracker/internal/service"
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

func TestFormatChecksPutsTheFixUnderTheProblem(t *testing.T) {
	lines := formatChecks([]service.Check{
		{Name: "platform", Status: service.OK, Detail: "systemd user service"},
		{Name: "service", Status: service.Fail, Detail: "inactive", Fix: "run: ai-usage service install"},
		{Name: "linger", Status: service.Warn, Detail: "off"},
	})
	want := []string{
		"ok    platform      systemd user service",
		"fail  service       inactive",
		"                    fix: run: ai-usage service install",
		"warn  linger        off",
		"",
		"1 problem(s), 1 warning(s)",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("linhas %q", lines)
	}
	if got := formatChecks([]service.Check{{Name: "a", Status: service.OK, Detail: "b"}}); got[len(got)-1] != "all checks passed" {
		t.Fatalf("resumo %q", got)
	}
}

func TestInstallWithoutAConfigurationExplainsTheNextStep(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	var out, errOut strings.Builder
	if code := serviceInstall(nil, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "ai-usage setup") {
		t.Fatalf("código %d saída %q", code, errOut.String())
	}
}
