package alert

import (
	"fmt"
	"time"

	"ai-usage-tracker/internal/contract"
)

type Intent struct {
	Key      string
	Kind     string
	Provider string
	Title    string
	Body     string
	Window   string
	Used     string
	ResetsAt time.Time
}

func FromSample(sample contract.Sample, threshold float64, now time.Time) []Intent {
	var intents []Intent
	for _, window := range sample.Windows {
		if window.Used >= threshold {
			intents = append(intents, Intent{
				Key:      fmt.Sprintf("threshold|%s|%s|%d", sample.Provider, window.Name, window.ResetsAt.Unix()),
				Kind:     "threshold",
				Provider: sample.Provider,
				Title:    "Limiar de uso",
				Body:     fmt.Sprintf("%s %s em %.0f%%.", label(sample.Provider), window.Name, window.Used),
				Window:   window.Name,
				Used:     fmt.Sprintf("%.0f", window.Used),
				ResetsAt: window.ResetsAt,
			})
		}
		until := window.ResetsAt.Sub(now)
		if until > 0 && until <= time.Hour {
			intents = append(intents, Intent{
				Key:      fmt.Sprintf("reset|%s|%s|%d", sample.Provider, window.Name, window.ResetsAt.Unix()),
				Kind:     "reset",
				Provider: sample.Provider,
				Title:    "Reset próximo",
				Body:     fmt.Sprintf("%s %s reinicia em até 60 minutos.", label(sample.Provider), window.Name),
				Window:   window.Name,
				Used:     fmt.Sprintf("%.0f", window.Used),
				ResetsAt: window.ResetsAt,
			})
		}
	}
	return intents
}

func Quiet(now time.Time, start string, end string, timezone string) (bool, error) {
	if start == "" && end == "" {
		return false, nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false, err
	}
	startMin, err := ClockMinutes(start)
	if err != nil {
		return false, err
	}
	endMin, err := ClockMinutes(end)
	if err != nil {
		return false, err
	}
	local := now.In(location)
	current := local.Hour()*60 + local.Minute()
	if startMin == endMin {
		return false, nil
	}
	if startMin < endMin {
		return current >= startMin && current < endMin, nil
	}
	return current >= startMin || current < endMin, nil
}

func ClockMinutes(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func ProviderLabel(provider string) string {
	return label(provider)
}

func label(provider string) string {
	switch provider {
	case contract.ProviderClaude:
		return "Claude"
	case contract.ProviderCodex:
		return "Codex"
	case contract.ProviderAntigravity:
		return "Antigravity"
	default:
		return provider
	}
}
