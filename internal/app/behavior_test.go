package app

import "testing"

func TestParseObservationMarker(t *testing.T) {
	tests := []struct {
		name      string
		marker    string
		wantCat   string
		wantField string
		wantValue string
		wantConf  int
		wantErr   bool
	}{
		{
			name:      "valid marker",
			marker:    "[BEHAVIORAL_OBSERVATION: category=tone, field=language, value=es, confidence=90]",
			wantCat:   "tone",
			wantField: "language",
			wantValue: "es",
			wantConf:  90,
		},
		{
			name:    "missing field key",
			marker:  "[BEHAVIORAL_OBSERVATION: category=tone, value=es, confidence=90]",
			wantErr: true,
		},
		{
			name:    "confidence out of range",
			marker:  "[BEHAVIORAL_OBSERVATION: category=tone, field=language, value=es, confidence=101]",
			wantErr: true,
		},
		{
			name:      "unknown key ignored",
			marker:    "[BEHAVIORAL_OBSERVATION: category=tone, field=language, value=es, confidence=90, extra=ignored]",
			wantCat:   "tone",
			wantField: "language",
			wantValue: "es",
			wantConf:  90,
		},
		{
			name:      "confidence zero valid",
			marker:    "[BEHAVIORAL_OBSERVATION: category=process, field=ask_before_mutation, value=true, confidence=0]",
			wantCat:   "process",
			wantField: "ask_before_mutation",
			wantValue: "true",
			wantConf:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, field, value, conf, err := parseObservationMarker(tt.marker)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cat != tt.wantCat || field != tt.wantField || value != tt.wantValue || conf != tt.wantConf {
				t.Fatalf("unexpected parse result: got (%q,%q,%q,%d)", cat, field, value, conf)
			}
		})
	}
}
