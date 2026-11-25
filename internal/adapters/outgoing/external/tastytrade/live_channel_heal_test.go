package tastytrade

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestChannelMuteStats_IsMute(t *testing.T) {
	now := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		stats channelMuteStats
		want  bool
	}{
		{
			name:  "menos de 50 miembros",
			stats: channelMuteStats{Members: 49, MuteMembers: 49, LastActivity: now.Add(-20 * time.Minute)},
			want:  false,
		},
		{
			name:  "LastActivity cero",
			stats: channelMuteStats{Members: 60, MuteMembers: 60},
			want:  false,
		},
		{
			name:  "actividad hace 10 min",
			stats: channelMuteStats{Members: 60, MuteMembers: 60, LastActivity: now.Add(-10 * time.Minute)},
			want:  false,
		},
		{
			name:  "60 miembros con 54 mudos y 20 min sin actividad",
			stats: channelMuteStats{Members: 60, MuteMembers: 54, LastActivity: now.Add(-20 * time.Minute)},
			want:  true,
		},
		{
			name:  "60 miembros con 53 mudos",
			stats: channelMuteStats{Members: 60, MuteMembers: 53, LastActivity: now.Add(-20 * time.Minute)},
			want:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.stats.isMute(now); got != tc.want {
				t.Fatalf("isMute() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsRegularSessionET(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"05:00 ET miercoles", time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC), false},
		{"09:29 ET", time.Date(2026, 11, 25, 14, 29, 0, 0, time.UTC), false},
		{"09:30 ET", time.Date(2026, 11, 25, 14, 30, 0, 0, time.UTC), true},
		{"12:00 ET", time.Date(2026, 11, 25, 17, 0, 0, 0, time.UTC), true},
		{"16:00 ET", time.Date(2026, 11, 25, 21, 0, 0, 0, time.UTC), false},
		{"sabado 12:00 ET", time.Date(2026, 11, 28, 17, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRegularSessionET(tc.now); got != tc.want {
				t.Fatalf("isRegularSessionET(%v) = %v, want %v", tc.now, got, tc.want)
			}
		})
	}
}

func TestHealMuteChannels_SkipsMuteChannelOutsideRegularSession(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx := context.Background()
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	ch.createdAt = now.Add(-1 * time.Hour)
	for i := 0; i < 60; i++ {
		sym := fmt.Sprintf("SYM%02d", i)
		ch.occupy(candleKey(sym, domain.M1))
		p.liveSubs[sym] = func(domain.Candle) {}
		p.noteLiveEvent(sym, now.Add(-20*time.Minute))
	}
	p.lastLiveEventAtUnixNano.Store(time.Now().UnixNano())

	if got := p.HealMuteChannels(ctx, now, 3); got != 0 {
		t.Fatalf("HealMuteChannels = %d, want 0", got)
	}
	if got := len(p.allocator.allChannels()); got != 1 {
		t.Fatalf("expected channel to remain, got %d channels", got)
	}
}

func TestHealMuteChannels_RecyclesMuteChannel(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx := context.Background()
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	now := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	ch.createdAt = now.Add(-1 * time.Hour)
	for i := 0; i < 60; i++ {
		sym := fmt.Sprintf("SYM%02d", i)
		ch.occupy(candleKey(sym, domain.M1))
		p.liveSubs[sym] = func(domain.Candle) {}
		p.noteLiveEvent(sym, now.Add(-20*time.Minute))
	}
	p.lastLiveEventAtUnixNano.Store(time.Now().UnixNano())

	if got := p.HealMuteChannels(ctx, now, 3); got != 1 {
		t.Fatalf("HealMuteChannels = %d, want 1", got)
	}
	if got := len(p.allocator.allChannels()); got != 0 {
		t.Fatalf("expected allocator.allChannels() empty, got %d", got)
	}
	if got := len(p.orphanedLiveSymbols()); got != 60 {
		t.Fatalf("expected 60 orphaned symbols, got %d", got)
	}
}
