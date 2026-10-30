package contract

import (
	"strings"
	"testing"
	"time"
)

func TestRejectsForbiddenAndUnknownFields(t *testing.T) {
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	body := `{
	  "sample_id": "smp_12345678",
	  "provider": "claude",
	  "collected_at": "2026-10-29T15:00:00Z",
	  "prompt": "CANARY_PROMPT",
	  "windows": [{"name":"five_hour","used":10,"limit":100,"unit":"percent","resets_at":"2026-10-29T20:00:00Z"}]
	}`
	_, err := DecodeSample([]byte(body), now)
	validation, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("erro %v", err)
	}
	if !strings.Contains(validation.Fields[0].Message, "proibido") && !containsField(validation, "prompt") {
		t.Fatalf("%#v", validation.Fields)
	}
	_, err = DecodeSample([]byte(`{"sample_id":"smp_12345678","provider":"claude","collected_at":"2026-10-29T15:00:00Z","note":"x","windows":[{"name":"five_hour","used":10,"limit":100,"unit":"percent","resets_at":"2026-10-29T20:00:00Z"}]}`), now)
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("campo livre aceito: %v", err)
	}
}

func TestPercentContract(t *testing.T) {
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	_, err := DecodeSample([]byte(`{"sample_id":"smp_12345678","provider":"claude","collected_at":"2026-10-29T15:00:00Z","windows":[{"name":"five_hour","used":10,"limit":500,"unit":"tokens","resets_at":"2026-10-29T20:00:00Z"}]}`), now)
	if err == nil {
		t.Fatal("aceito teto inventado")
	}
	sample, err := DecodeSample([]byte(`{"sample_id":"smp_12345678","provider":"codex","collected_at":"2026-10-29T15:00:00Z","plan":"pro","windows":[{"name":"five_hour","used":0,"limit":100,"unit":"percent","resets_at":"2026-10-29T20:00:00Z"}]}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Windows[0].Used != 0 {
		t.Fatal("zero virou ausência")
	}
}

func containsField(err *ValidationError, field string) bool {
	for _, item := range err.Fields {
		if strings.Contains(item.Field, field) {
			return true
		}
	}
	return false
}
