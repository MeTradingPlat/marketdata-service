package tastytrade

import "testing"

func TestHandleLiveTradeVolume_NoteTradeOnlyOnIncrease(t *testing.T) {
	p := newTestPool()

	v100, v50, v150 := 100.0, 50.0, 150.0
	cases := []struct {
		name      string
		dayVolume *float64
		wantTrade bool
		wantVol   float64
	}{
		{name: "first value", dayVolume: &v100, wantTrade: false, wantVol: 100},
		{name: "nil volume keeps value", dayVolume: nil, wantTrade: false, wantVol: 100},
		{name: "equal value", dayVolume: &v100, wantTrade: false, wantVol: 100},
		{name: "new day reset", dayVolume: &v50, wantTrade: false, wantVol: 50},
		{name: "increased value", dayVolume: &v150, wantTrade: true, wantVol: 150},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p.activityMu.Lock()
			p.ensureActivityMaps()
			delete(p.lastTradeAt, "NVDA")
			p.activityMu.Unlock()

			p.handleLiveTradeVolume(rawTradeEvent{Symbol: "NVDA", DayVolume: tc.dayVolume})

			p.activityMu.Lock()
			gotTrade := !p.lastTradeAt["NVDA"].IsZero()
			p.activityMu.Unlock()
			if gotTrade != tc.wantTrade {
				t.Fatalf("gotTrade=%v, want %v", gotTrade, tc.wantTrade)
			}
			entry, ok := p.LiveDayVolume("NVDA")
			if !ok || entry.Regular == nil || *entry.Regular != tc.wantVol {
				t.Fatalf("got stored volume %+v, want %v", entry, tc.wantVol)
			}
		})
	}
}
