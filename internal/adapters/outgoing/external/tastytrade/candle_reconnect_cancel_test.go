package tastytrade

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_HandleConnectionReconnect_CanceledCtxClearsPendingRevive(t *testing.T) {
	var dialCalls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	p := NewCandlePool(func(context.Context) (*DxLinkConn, error) {
		dialCalls.Add(1)
		cancel()
		return nil, errors.New("dial failed")
	}, 2)

	pc := newPooledConnection(NewDxLinkConn(func() string { return "" }, func() string { return "" }))
	ch := newPooledChannel(&dxLinkChannel{id: 1})
	ch.channel.retired.Store(true)
	p.liveMu.Lock()
	for _, sym := range []string{"AAA", "BBB", "CCC"} {
		ch.occupy(candleKey(sym, domain.M1))
		p.liveSubs[sym] = func(domain.Candle) {}
	}
	p.liveMu.Unlock()
	pc.addChannel(ch)
	p.activityMu.Lock()
	for _, sym := range []string{"AAA", "BBB", "CCC"} {
		p.lastEventAt[sym] = time.Now()
	}
	p.activityMu.Unlock()

	p.handleConnectionReconnect(ctx, pc)

	if got := dialCalls.Load(); got != 1 {
		t.Fatalf("expected 1 dial attempt before stopping on canceled ctx, got %d", got)
	}
	p.activityMu.Lock()
	remainingRevive := len(p.pendingRevive)
	p.activityMu.Unlock()
	if remainingRevive != 0 {
		t.Fatalf("expected pendingRevive cleared, got %d entries", remainingRevive)
	}
	p.liveMu.Lock()
	subs := len(p.liveSubs)
	p.liveMu.Unlock()
	if subs != 3 {
		t.Fatalf("expected all 3 symbols kept in liveSubs, got %d", subs)
	}
}
