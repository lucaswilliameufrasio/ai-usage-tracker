package freshness

import (
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
)

func TestUnavailableReportHidesLiveReading(t *testing.T) {
	now := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	sample := sampleAt(now.Add(-time.Minute), 10)
	report := &contract.SourceReport{
		Provider:   contract.ProviderCodex,
		ObservedAt: now,
		Status:     contract.StatusUnavailable,
		ReasonCode: contract.ReasonLimitsAbsent,
	}
	card := Build(contract.ProviderCodex, &sample, report, []contract.Sample{sample}, now, 30*time.Minute)
	if card.Freshness != Unavailable || card.Projection != nil {
		t.Fatalf("card %+v", card)
	}
	if card.LastSample == nil || card.LastSample.SampleID != sample.SampleID {
		t.Fatal("última amostra deveria permanecer visível")
	}
}

func TestProjectionUsesTwoPointsAndStaleQuality(t *testing.T) {
	start := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	older := sampleAt(start, 20)
	newer := sampleAt(start.Add(time.Hour), 40)
	now := start.Add(2 * time.Hour)
	card := Build(contract.ProviderClaude, &newer, nil, []contract.Sample{newer, older}, now, time.Minute)
	if card.Freshness != Stale {
		t.Fatalf("freshness %s", card.Freshness)
	}
	if card.Projection == nil {
		t.Fatal("projeção ausente")
	}
	if card.Projection.DataQuality != Stale || card.Projection.EstimatedUsed != 80 {
		t.Fatalf("projeção %+v", card.Projection)
	}
}

func TestSinglePointHasNoProjection(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	sample := sampleAt(now, 15)
	card := Build(contract.ProviderClaude, &sample, nil, []contract.Sample{sample}, now, time.Hour)
	if card.Freshness != Current || card.Projection != nil {
		t.Fatalf("card %+v", card)
	}
}

func sampleAt(at time.Time, used float64) contract.Sample {
	return contract.Sample{
		SampleID:    "smp_fixture_one",
		Provider:    contract.ProviderClaude,
		CollectedAt: at,
		Windows: []contract.Window{{
			Name:     "five_hour",
			Used:     used,
			Limit:    100,
			Unit:     contract.UnitPercent,
			ResetsAt: at.Add(2 * time.Hour),
		}},
	}
}
