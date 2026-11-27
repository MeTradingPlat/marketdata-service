package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestMoveLiveSymbolChannel_AlreadyMovingAndReusable(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 3)
	p.SetMaxChannelsPerConn(1)

	ctx := context.Background()
	now := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	if err := p.SubscribeLive(ctx, "NVDA", now, func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	rec.waitCount(t, 3)
	oldCh := p.liveChannelFor("NVDA")
	before := len(rec.snapshot())

	p.movingSymbols.Store("NVDA", struct{}{})
	if _, err := p.moveLiveSymbolChannel(ctx, "NVDA", oldCh, now, 0, now); !errors.Is(err, errMoveInProgress) {
		t.Fatalf("err = %v, want errMoveInProgress", err)
	}
	if len(rec.snapshot()) != before || p.liveChannelFor("NVDA") != oldCh || len(p.allocator.allChannels()) != 1 {
		t.Fatal("expected no server IO or reservation change while moving")
	}
	if ok, failed := p.reviveSilentSymbol(ctx, "NVDA", now); ok || failed || p.pendingRevive["NVDA"].attempts != 0 {
		t.Fatalf("reviveSilentSymbol = (%v, %v), attempts=%d", ok, failed, p.pendingRevive["NVDA"].attempts)
	}
	if ok, err := p.moveSilentSymbolChannel(ctx, "NVDA", oldCh, now); ok || err != nil || len(p.silentHealHistory["NVDA"]) != 0 {
		t.Fatalf("moveSilentSymbolChannel = (%v, %v), history=%d", ok, err, len(p.silentHealHistory["NVDA"]))
	}
	p.movingSymbols.Delete("NVDA")

	if _, err := p.moveLiveSymbolChannel(ctx, "NVDA", oldCh, now, 99, now); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on stale epoch, got %v", err)
	}
	if newCh, err := p.moveLiveSymbolChannel(ctx, "NVDA", oldCh, now, 0, now); err != nil || newCh == oldCh {
		t.Fatalf("expected move after error to succeed, ch=%v err=%v", newCh, err)
	}
}
