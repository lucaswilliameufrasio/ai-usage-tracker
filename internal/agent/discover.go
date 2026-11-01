package agent

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-usage-tracker/internal/collect"
	"ai-usage-tracker/internal/contract"
)

type Place struct {
	Path   string
	Detail string
}

func Suggest(home string, now time.Time) (Place, Place) {
	claudePath := filepath.Join(home, ".claude.json")
	codexPath := filepath.Join(home, ".codex", "sessions")
	return Place{Path: existingPath(claudePath), Detail: DescribeClaude(claudePath, now)}, Place{Path: existingDir(codexPath), Detail: DescribeCodex(codexPath, now)}
}

func DescribeClaude(path string, now time.Time) string {
	if path == "" {
		return "No file suggested."
	}
	if _, err := os.Stat(path); err != nil {
		return "That path does not exist."
	}
	batch, err := collect.Read(collect.Request{ClaudeFile: path, Now: now})
	if err != nil {
		return "Could not read that path."
	}
	return describe(batch, contract.ProviderClaude, "")
}

func DescribeCodex(path string, now time.Time) string {
	if path == "" {
		return "No folder suggested."
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "That path does not exist."
	}
	batch, err := collect.Read(collect.Request{CodexDir: path, Now: now})
	if err != nil {
		return "Could not read that path."
	}
	return describe(batch, contract.ProviderCodex, path)
}

func describe(batch collect.Batch, provider string, codexDir string) string {
	for _, sample := range batch.Samples {
		if sample.Provider != provider {
			continue
		}
		when := sample.CollectedAt
		if sample.SourceObservedAt != nil {
			when = sample.SourceObservedAt.UTC()
		}
		detail := "Limit found, observed at " + when.In(time.Local).Format("2006-01-02 15:04 MST") + "."
		if provider == contract.ProviderCodex && newerSessionWithoutLimits(codexDir, when) {
			detail += " Newer sessions carried no rate_limits."
		}
		return detail
	}
	for _, report := range batch.Reports {
		if report.Provider != provider {
			continue
		}
		return reasonText(report.ReasonCode)
	}
	return "No limit recorded at that path."
}

func reasonText(code string) string {
	if code == contract.ReasonSourceUnreadable {
		return "Could not read that path."
	}
	if code == contract.ReasonResetUnknown {
		return "The limit has no reset time."
	}
	return "No limit recorded at that path."
}

func newerSessionWithoutLimits(dir string, observed time.Time) bool {
	newest, ok := newestRollout(dir)
	if !ok {
		return false
	}
	return newest.After(observed.Add(time.Hour))
}

func newestRollout(dir string) (time.Time, bool) {
	var newest time.Time
	found := false
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "rollout") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if !found || info.ModTime().After(newest) {
			newest = info.ModTime()
			found = true
		}
		return nil
	})
	return newest, found
}

func existingPath(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return path
}

func existingDir(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return path
}
