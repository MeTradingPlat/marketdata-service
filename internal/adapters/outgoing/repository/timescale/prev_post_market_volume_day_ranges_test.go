package timescale

import (
	"testing"
	"time"
)

func TestPreviousPostMarketWindows(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		beforeUTC string
		wantFirst time.Time
		wantStart time.Time
		wantPost  time.Time
		wantEnd   time.Time
	}{
		{"after 20 ET same day", "2026-11-27T02:04:00Z", time.Date(2026, 11, 26, 0, 0, 0, 0, loc), time.Date(2026, 11, 26, 4, 0, 0, 0, loc), time.Date(2026, 11, 26, 16, 0, 0, 0, loc), time.Date(2026, 11, 26, 20, 0, 0, 0, loc)},
		{"early morning ET prev day", "2026-11-27T08:46:00Z", time.Date(2026, 11, 26, 0, 0, 0, 0, loc), time.Date(2026, 11, 26, 4, 0, 0, 0, loc), time.Date(2026, 11, 26, 16, 0, 0, 0, loc), time.Date(2026, 11, 26, 20, 0, 0, 0, loc)},
		{"21 ET uses same ET day", "2026-11-28T02:00:00Z", time.Date(2026, 11, 27, 0, 0, 0, 0, loc), time.Date(2026, 11, 27, 4, 0, 0, 0, loc), time.Date(2026, 11, 27, 13, 0, 0, 0, loc), time.Date(2026, 11, 27, 17, 0, 0, 0, loc)},
		{"early close 17:30 ET uses same day", "2026-11-27T22:30:00Z", time.Date(2026, 11, 27, 0, 0, 0, 0, loc), time.Date(2026, 11, 27, 4, 0, 0, 0, loc), time.Date(2026, 11, 27, 13, 0, 0, 0, loc), time.Date(2026, 11, 27, 17, 0, 0, 0, loc)},
		{"early close 16:00 ET still in post uses prev day", "2026-11-27T21:00:00Z", time.Date(2026, 11, 26, 0, 0, 0, 0, loc), time.Date(2026, 11, 26, 4, 0, 0, 0, loc), time.Date(2026, 11, 26, 16, 0, 0, 0, loc), time.Date(2026, 11, 26, 20, 0, 0, 0, loc)},
		{"17 ET on normal day still in post uses prev day", "2026-11-30T22:00:00Z", time.Date(2026, 11, 29, 0, 0, 0, 0, loc), time.Date(2026, 11, 29, 4, 0, 0, 0, loc), time.Date(2026, 11, 29, 16, 0, 0, 0, loc), time.Date(2026, 11, 29, 20, 0, 0, 0, loc)},
		{"after DST fall back 2026-11-02", "2026-11-03T12:00:00Z", time.Date(2026, 11, 2, 0, 0, 0, 0, loc), time.Date(2026, 11, 2, 4, 0, 0, 0, loc), time.Date(2026, 11, 2, 16, 0, 0, 0, loc), time.Date(2026, 11, 2, 20, 0, 0, 0, loc)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, err := time.Parse(time.RFC3339, tt.beforeUTC)
			if err != nil {
				t.Fatal(err)
			}
			got := previousPostMarketWindows(before, 2, loc)
			if len(got) != 2 {
				t.Fatalf("len = %d, want 2", len(got))
			}
			if !got[0].start.Equal(tt.wantStart) || !got[0].postStart.Equal(tt.wantPost) || !got[0].end.Equal(tt.wantEnd) {
				t.Fatalf("got[0] = %+v, want [%v %v %v]", got[0], tt.wantStart, tt.wantPost, tt.wantEnd)
			}
		})
	}
}
