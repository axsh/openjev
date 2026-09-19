package decision

import (
	"strings"
	"testing"

	"openjev/features/decision-test/internal/domain"
)

func twoCriteria() []domain.Criterion {
	return []domain.Criterion{
		{Key: "north", Text: "north: Route north"},
		{Key: "south", Text: "south: Route south"},
	}
}

func TestValidateGeneration(t *testing.T) {
	ok := `{"A: north: Route north": 0.65, "B: south: Route south": 0.35}`
	tests := []struct {
		name    string
		text    string
		wantOK  bool
		wantErr string
		wantKey string
	}{
		{name: "ok", text: ok, wantOK: true, wantKey: "north"},
		{name: "think", text: "<think>secret</think>\n" + ok, wantOK: true, wantKey: "north"},
		{name: "not_object", text: `[1,2]`, wantErr: "expected one JSON object"},
		{name: "missing_key", text: `{"A: north: Route north": 1}`, wantErr: "expected one probability for every exact option key"},
		{name: "extra_key", text: `{"A: north: Route north": 0.5, "B: south: Route south": 0.5, "C: extra: Extra": 0}`, wantErr: "expected one probability for every exact option key"},
		{name: "bad_number", text: `{"A: north: Route north": "no", "B: south: Route south": 1}`, wantErr: "probabilities must be numbers from 0 to 1"},
		{name: "over_one", text: `{"A: north: Route north": 1.1, "B: south: Route south": -0.1}`, wantErr: "probabilities must be numbers from 0 to 1"},
		{name: "sum", text: `{"A: north: Route north": 0.9, "B: south: Route south": 0.2}`, wantErr: "probabilities must sum to 1"},
		{name: "sum_edge", text: `{"A: north: Route north": 0.51, "B: south: Route south": 0.51}`, wantOK: true, wantKey: "north"},
		{name: "sum_over_edge", text: `{"A: north: Route north": 0.521, "B: south: Route south": 0.5}`, wantErr: "probabilities must sum to 1"},
		{name: "tie", text: `{"A: north: Route north": 0.5, "B: south: Route south": 0.5}`, wantOK: true, wantKey: "north"},
		{name: "markdown", text: "```json\n" + ok + "\n```", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateGeneration(tt.text, twoCriteria())
			if tt.wantOK {
				if !got.Valid || got.ChoiceKey != tt.wantKey {
					t.Fatalf("%+v", got)
				}
				return
			}
			if got.Valid {
				t.Fatal("expected invalid")
			}
			if tt.wantErr != "" && !strings.Contains(got.ValidationError, tt.wantErr) {
				t.Fatalf("error %q", got.ValidationError)
			}
		})
	}
}
