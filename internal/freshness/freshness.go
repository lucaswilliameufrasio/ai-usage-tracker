package freshness

import (
	"math"
	"strings"
	"time"

	"ai-usage-tracker/internal/contract"
)

const (
	Current     = "current"
	Stale       = "stale"
	Empty       = "empty"
	Unavailable = "unavailable"
)

type Projection struct {
	Window        string    `json:"window"`
	EstimatedUsed float64   `json:"estimated_used"`
	ResetsAt      time.Time `json:"resets_at"`
	DataQuality   string    `json:"data_quality"`
}

type Card struct {
	Provider   string
	Freshness  string
	ReasonCode string
	ObservedAt *time.Time
	LastSample *contract.Sample
	Projection *Projection
}

func Build(provider string, sample *contract.Sample, report *contract.SourceReport, history []contract.Sample, now time.Time, staleAfter time.Duration) Card {
	card := Card{Provider: provider, Freshness: Empty}
	if sample == nil && report == nil {
		return card
	}
	reportWins := report != nil && (sample == nil || report.ObservedAt.After(sample.CollectedAt))
	if reportWins {
		observed := report.ObservedAt.UTC()
		card.Freshness = Unavailable
		card.ReasonCode = report.ReasonCode
		card.ObservedAt = &observed
		card.LastSample = sample
		return card
	}
	observed := sample.CollectedAt.UTC()
	card.ObservedAt = &observed
	card.LastSample = sample
	if now.Sub(sample.CollectedAt) > staleAfter {
		card.Freshness = Stale
	} else {
		card.Freshness = Current
	}
	card.Projection = project(sample, history, card.Freshness)
	return card
}

func project(latest *contract.Sample, history []contract.Sample, quality string) *Projection {
	if latest == nil || len(latest.Windows) == 0 {
		return nil
	}
	name := latest.Windows[0].Name
	for _, window := range latest.Windows {
		if window.Name == "five_hour" || strings.HasSuffix(window.Name, "_five_hour") {
			name = window.Name
			break
		}
	}
	var used []float64
	var at []time.Time
	var resets time.Time
	for _, sample := range history {
		for _, window := range sample.Windows {
			if window.Name != name {
				continue
			}
			used = append(used, window.Used)
			at = append(at, sample.CollectedAt)
			if len(used) == 1 {
				resets = window.ResetsAt
			}
			break
		}
		if len(used) == 2 {
			break
		}
	}
	if len(used) < 2 {
		return nil
	}
	dt := at[0].Sub(at[1]).Seconds()
	if dt <= 0 {
		return nil
	}
	remaining := resets.Sub(at[0]).Seconds()
	if remaining <= 0 {
		return nil
	}
	estimated := used[0] + ((used[0]-used[1])/dt)*remaining
	if estimated < 0 {
		estimated = 0
	}
	return &Projection{
		Window:        name,
		EstimatedUsed: math.Round(estimated*10) / 10,
		ResetsAt:      resets.UTC(),
		DataQuality:   quality,
	}
}
