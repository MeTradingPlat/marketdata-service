package domain

import (
	"testing"
	"time"
)

func TestNYSESessionHoursFor(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		day         time.Time
		wantPreOpen time.Time
		wantOpen    time.Time
		wantClose   time.Time
		wantPostEnd time.Time
	}{
		{
			name:        "black friday 2026 early close",
			day:         time.Date(2026, 11, 27, 10, 15, 0, 0, loc),
			wantPreOpen: time.Date(2026, 11, 27, 4, 0, 0, 0, loc),
			wantOpen:    time.Date(2026, 11, 27, 9, 30, 0, 0, loc),
			wantClose:   time.Date(2026, 11, 27, 13, 0, 0, 0, loc),
			wantPostEnd: time.Date(2026, 11, 27, 17, 0, 0, 0, loc),
		},
		{
			name:        "regular monday 2026-11-30",
			day:         time.Date(2026, 11, 30, 0, 0, 0, 0, loc),
			wantPreOpen: time.Date(2026, 11, 30, 4, 0, 0, 0, loc),
			wantOpen:    time.Date(2026, 11, 30, 9, 30, 0, 0, loc),
			wantClose:   time.Date(2026, 11, 30, 16, 0, 0, 0, loc),
			wantPostEnd: time.Date(2026, 11, 30, 20, 0, 0, 0, loc),
		},
		{
			name:        "christmas eve 2026 early close",
			day:         time.Date(2026, 12, 24, 15, 0, 0, 0, loc),
			wantPreOpen: time.Date(2026, 12, 24, 4, 0, 0, 0, loc),
			wantOpen:    time.Date(2026, 12, 24, 9, 30, 0, 0, loc),
			wantClose:   time.Date(2026, 12, 24, 13, 0, 0, 0, loc),
			wantPostEnd: time.Date(2026, 12, 24, 17, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NYSESessionHoursFor(tt.day)
			if !got.PreOpen.Equal(tt.wantPreOpen) || !got.Open.Equal(tt.wantOpen) ||
				!got.Close.Equal(tt.wantClose) || !got.PostEnd.Equal(tt.wantPostEnd) {
				t.Fatalf("got %+v, want PreOpen=%v Open=%v Close=%v PostEnd=%v",
					got, tt.wantPreOpen, tt.wantOpen, tt.wantClose, tt.wantPostEnd)
			}
			if got.Close.Location() != loc || got.PostEnd.Location() != loc {
				t.Fatalf("expected location %v, got Close=%v PostEnd=%v", loc, got.Close.Location(), got.PostEnd.Location())
			}
		})
	}
}
