package secedgar

import (
	"testing"
	"time"
)

func TestCurrentAndPreviousQuarterInstant(t *testing.T) {
	tests := []struct {
		name         string
		now          time.Time
		wantCurrent  string
		wantPrevious string
	}{
		{
			name:         "january 1 is Q1, previous is Q4 of prior year",
			now:          time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			wantCurrent:  "CY2026Q1I",
			wantPrevious: "CY2025Q4I",
		},
		{
			name:         "march 31 stays in Q1",
			now:          time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC),
			wantCurrent:  "CY2026Q1I",
			wantPrevious: "CY2025Q4I",
		},
		{
			name:         "april 1 rolls into Q2",
			now:          time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			wantCurrent:  "CY2026Q2I",
			wantPrevious: "CY2026Q1I",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current, previous := currentAndPreviousQuarterInstant(tt.now)
			if current != tt.wantCurrent {
				t.Errorf("current = %q, want %q", current, tt.wantCurrent)
			}
			if previous != tt.wantPrevious {
				t.Errorf("previous = %q, want %q", previous, tt.wantPrevious)
			}
		})
	}
}

func TestMatchFrameEntities(t *testing.T) {
	const (
		belowMinCIK = 1111111111
		staleCIK    = 2222222222
		validCIK    = 3333333333
		validShares = int64(34_000_000)
	)

	recent := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	stale := time.Now().AddDate(-3, 0, 0).Format("2006-01-02")

	entities := []frameEntity{
		{CIK: belowMinCIK, EntityName: "Tiny Class B", Val: minPlausibleShares - 1, End: recent},
		{CIK: staleCIK, EntityName: "Stopped Reporting", Val: 20_000_000, End: stale},
		{CIK: validCIK, EntityName: "Valid Issuer", Val: validShares, End: recent},
	}

	cikToSymbols := map[string][]string{
		"1111111111": {"TINY"},
		"2222222222": {"OLD"},
		"3333333333": {"VLD", "VLDW"},
		"4444444444": {"UNSEEN"},
	}
	resolvedCiks := map[string]bool{}
	shares := map[string]int64{}

	matchFrameEntities(entities, cikToSymbols, resolvedCiks, shares)

	if _, ok := shares["TINY"]; ok {
		t.Errorf("below-minPlausibleShares entity should be discarded, got %d", shares["TINY"])
	}
	if _, ok := shares["OLD"]; ok {
		t.Errorf("stale entity should be discarded, got %d", shares["OLD"])
	}
	if got := shares["VLD"]; got != validShares {
		t.Errorf("VLD = %d, want %d", got, validShares)
	}
	if got := shares["VLDW"]; got != validShares {
		t.Errorf("VLDW = %d, want %d (all symbols of the CIK)", got, validShares)
	}
	if !resolvedCiks["3333333333"] {
		t.Error("valid CIK should be marked resolved")
	}
	if resolvedCiks["1111111111"] || resolvedCiks["2222222222"] {
		t.Error("discarded entities must not be marked resolved")
	}
}
