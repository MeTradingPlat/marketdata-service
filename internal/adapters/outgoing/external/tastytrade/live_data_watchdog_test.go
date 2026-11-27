package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCheckLiveData(t *testing.T) {
	inSession := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		now         time.Time
		subs        int
		lastTrigger time.Time
		wantTrigger bool
	}{
		{"holiday", time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC), 1, time.Time{}, false},
		{"after early close", time.Date(2026, 11, 27, 22, 5, 0, 0, time.UTC), 1, time.Time{}, false},
		{"just opened 04:01 ET", time.Date(2026, 11, 25, 9, 1, 0, 0, time.UTC), 1, time.Time{}, false},
		{"no subscriptions", inSession, 0, time.Time{}, false},
		{"cooldown", inSession, 1, inSession.Add(-2 * time.Minute), false},
		{"real silence", inSession, 1, time.Time{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestPool()
			p.allocator = newChannelAllocator(nil, nil, nil, 1)
			for i := 0; i < tc.subs; i++ {
				p.liveSubs["AAPL"] = func(domain.Candle) {}
			}
			p.lastLiveEventAtUnixNano.Store(time.Now().Add(-5 * time.Minute).UnixNano())
			last := tc.lastTrigger
			checkLiveData(context.Background(), p, tc.now, &last)
			if got := last.Equal(tc.now); got != tc.wantTrigger {
				t.Fatalf("triggered=%v, want %v", got, tc.wantTrigger)
			}
		})
	}
}

func TestRunWatchdogTick_DropsReviveOutsideSession(t *testing.T) {
	p := newTestPool()
	p.ensureActivityMaps()
	holiday := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	p.pendingRevive["AAPL"] = reviveState{attempts: 2, lastAttemptAt: holiday.Add(-10 * time.Minute)}
	var lastSess, lastTrig time.Time
	runWatchdogTick(context.Background(), p, holiday, &lastSess, &lastTrig)
	if len(p.pendingRevive) != 0 {
		t.Fatalf("expected pendingRevive dropped on holiday, got %d", len(p.pendingRevive))
	}
}

func TestStartLiveDataWatchdog_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	StartLiveDataWatchdog(ctx, newTestPool())
	cancel()
}
