package tastytrade

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestHealSilentSymbols_MovesChannelAndIgnoresIlliquid(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(2)

	ctx := context.Background()
	if err := p.SubscribeLive(ctx, "NVDA", time.Now(), func(domain.Candle) {}, func(domain.Candle) {}); err != nil {
		t.Fatalf("SubscribeLive NVDA: %v", err)
	}
	if err := p.SubscribeLive(ctx, "ILLQ", time.Now(), func(domain.Candle) {}, func(domain.Candle) {}); err != nil {
		t.Fatalf("SubscribeLive ILLQ: %v", err)
	}
	oldCh := p.liveChannelFor("NVDA")
	for i := 0; i < channelCapacity; i++ {
		oldCh.occupy(fmt.Sprintf("FILL%03d", i))
	}

	now := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	p.lastLiveEventAtUnixNano.Store(time.Now().Add(-time.Second).UnixNano())
	p.noteNewBar("NVDA", now.Add(-6*time.Minute), now.Add(-6*time.Minute))
	p.noteTrade("NVDA", now.Add(-1*time.Minute))
	p.noteNewBar("ILLQ", now.Add(-6*time.Minute), now.Add(-6*time.Minute))

	if got := p.HealSilentSymbols(ctx, time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC), 100); got != 0 {
		t.Fatalf("expected 0 outside regular session, got %d", got)
	}
	if got := p.HealSilentSymbols(ctx, now, 100); got != 1 {
		t.Fatalf("expected 1 healed symbol, got %d", got)
	}
	if got := p.liveChannelFor("NVDA"); got == nil || got == oldCh {
		t.Fatal("expected NVDA to move to a different channel")
	}
	if got := p.liveChannelFor("ILLQ"); got != oldCh {
		t.Fatal("expected ILLQ to stay on original channel")
	}
}

func TestCandlePool_MoveSilentSymbolChannel_NoCandlesUses10mFallback(t *testing.T) {
	wsURL, subCh := startSubRecordingDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(2)

	ctx := context.Background()
	if err := p.SubscribeLive(ctx, "NVDA", time.Now(), func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive NVDA: %v", err)
	}
	<-subCh

	oldCh := p.liveChannelFor("NVDA")
	for i := 0; i < channelCapacity; i++ {
		oldCh.occupy(fmt.Sprintf("FILL%03d", i))
	}

	now := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	moved, err := p.moveSilentSymbolChannel(ctx, "NVDA", oldCh, now)
	if err != nil || !moved {
		t.Fatalf("moveSilentSymbolChannel: moved=%v err=%v", moved, err)
	}
	want := now.Add(-10 * time.Minute).UnixMilli()
	if got := <-subCh; got != want {
		t.Fatalf("expected fromTime %d (now-10m), got %d", want, got)
	}
}
