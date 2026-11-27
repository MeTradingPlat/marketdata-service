package timescale

import (
	"testing"
	"time"
)

func TestPreviousSessionCloseWindows(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		before    time.Time
		days      int
		wantFirst sessionCloseWindow
		checkIdx  int
		wantAtIdx sessionCloseWindow
	}{
		{
			name:      "2026-11-30 09:30 ET starts at 2026-11-29 and 27th is 12:58-13:01",
			before:    time.Date(2026, 11, 30, 9, 30, 0, 0, loc),
			days:      4,
			wantFirst: sessionCloseWindow{time.Date(2026, 11, 29, 15, 58, 0, 0, loc), time.Date(2026, 11, 29, 16, 1, 0, 0, loc)},
			checkIdx:  2,
			wantAtIdx: sessionCloseWindow{time.Date(2026, 11, 27, 12, 58, 0, 0, loc), time.Date(2026, 11, 27, 13, 1, 0, 0, loc)},
		},
		{
			name:      "2026-11-28T01:40Z (20:40 ET on 27th) starts at 2026-11-27",
			before:    time.Date(2026, 11, 28, 1, 40, 0, 0, time.UTC),
			days:      2,
			wantFirst: sessionCloseWindow{time.Date(2026, 11, 27, 12, 58, 0, 0, loc), time.Date(2026, 11, 27, 13, 1, 0, 0, loc)},
		},
		{
			name:      "2026-11-27 09:30 ET starts at 2026-11-26",
			before:    time.Date(2026, 11, 27, 9, 30, 0, 0, loc),
			days:      2,
			wantFirst: sessionCloseWindow{time.Date(2026, 11, 26, 15, 58, 0, 0, loc), time.Date(2026, 11, 26, 16, 1, 0, 0, loc)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := previousSessionCloseWindows(tt.before, tt.days, loc)
			if len(got) != tt.days {
				t.Fatalf("len = %d, want %d", len(got), tt.days)
			}
			if !got[0].from.Equal(tt.wantFirst.from) || !got[0].to.Equal(tt.wantFirst.to) {
				t.Fatalf("got[0] = [%v, %v), want [%v, %v)", got[0].from, got[0].to, tt.wantFirst.from, tt.wantFirst.to)
			}
			if tt.checkIdx > 0 {
				w := got[tt.checkIdx]
				if !w.from.Equal(tt.wantAtIdx.from) || !w.to.Equal(tt.wantAtIdx.to) {
					t.Fatalf("got[%d] = [%v, %v), want [%v, %v)", tt.checkIdx, w.from, w.to, tt.wantAtIdx.from, tt.wantAtIdx.to)
				}
			}
		})
	}
}
