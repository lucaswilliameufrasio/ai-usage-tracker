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
		return "Nenhum arquivo sugerido."
	}
	if _, err := os.Stat(path); err != nil {
		return "Esse caminho não existe."
	}
	batch, err := collect.Read(collect.Request{ClaudeFile: path, Now: now})
	if err != nil {
		return "Não foi possível ler esse caminho."
	}
	return describe(batch, contract.ProviderClaude, "")
}

func DescribeCodex(path string, now time.Time) string {
	if path == "" {
		return "Nenhuma pasta sugerida."
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "Esse caminho não existe."
	}
	batch, err := collect.Read(collect.Request{CodexDir: path, Now: now})
	if err != nil {
		return "Não foi possível ler esse caminho."
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
		detail := "Limite encontrado, observado em " + when.Format("02/01/2006 15:04 UTC") + "."
		if provider == contract.ProviderCodex && newerSessionWithoutLimits(codexDir, when) {
			detail += " Sessões mais novas não trouxeram rate_limits."
		}
		return detail
	}
	for _, report := range batch.Reports {
		if report.Provider != provider {
			continue
		}
		return reasonText(report.ReasonCode)
	}
	return "Nenhum limite gravado nesse caminho."
}

func reasonText(code string) string {
	if code == contract.ReasonSourceUnreadable {
		return "Não foi possível ler esse caminho."
	}
	if code == contract.ReasonResetUnknown {
		return "O limite está sem horário de reset."
	}
	return "Nenhum limite gravado nesse caminho."
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
