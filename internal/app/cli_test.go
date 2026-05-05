package app

import (
	"strings"
	"testing"
	"time"
)

func TestParsePositiveID(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"valid 1", "1", 1, false},
		{"valid 42", "42", 42, false},
		{"valid with spaces", "  7  ", 7, false},
		{"zero rejected", "0", 0, true},
		{"negative rejected", "-1", 0, true},
		{"non-numeric rejected", "abc", 0, true},
		{"empty rejected", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePositiveID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestFormatNote_RendersIDContentAndUTCTimestamps(t *testing.T) {
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2024, 1, 3, 6, 7, 8, 0, time.UTC)
	out := FormatNote(MemoryItem{ID: 9, Content: "hello", CreatedAt: created, UpdatedAt: updated})

	if !strings.Contains(out, "#9") {
		t.Fatalf("expected id marker #9 in %q", out)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected content in %q", out)
	}
	if !strings.Contains(out, "2024-01-02T03:04:05Z") {
		t.Fatalf("expected created_at UTC in %q", out)
	}
	if !strings.Contains(out, "2024-01-03T06:07:08Z") {
		t.Fatalf("expected updated_at UTC in %q", out)
	}
}

func TestFormatNote_NormalisesNonUTCTimestamps(t *testing.T) {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Skipf("tz not available: %v", err)
	}
	// 03:04:05 UTC == 00:04:05 in Buenos Aires (UTC-3)
	created := time.Date(2024, 1, 2, 0, 4, 5, 0, loc)
	out := FormatNote(MemoryItem{ID: 1, Content: "x", CreatedAt: created, UpdatedAt: created})
	if !strings.Contains(out, "2024-01-02T03:04:05Z") {
		t.Fatalf("expected timestamp normalised to UTC, got %q", out)
	}
}
