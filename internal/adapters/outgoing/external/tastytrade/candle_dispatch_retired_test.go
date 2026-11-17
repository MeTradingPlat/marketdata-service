package tastytrade

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func newDispatchPool() *CandlePool {
	return &CandlePool{dispatch: make(map[string]dispatchEntry)}
}

func m1Event(symbol string) rawCandleEvent {
	return rawCandleEvent{Symbol: symbol + "{=1m}"}
}

func TestRetiredHandlerEventsCountAsLateNotOrphan(t *testing.T) {
	p := newDispatchPool()
	id := p.registerDispatch("SHPH", domain.M1, "history-batch", func(rawCandleEvent) {})
	p.unregisterDispatchIfCurrent("SHPH", domain.M1, id, "history-batch")

	p.routeEvent(m1Event("SHPH"))
	p.routeEvent(m1Event("SHPH"))

	if got := atomic.LoadInt64(&p.lateEvents); got != 2 {
		t.Fatalf("lateEvents = %d, want 2", got)
	}
	if got := atomic.LoadInt64(&p.orphanEvents); got != 0 {
		t.Fatalf("orphanEvents = %d, want 0", got)
	}
}

func TestNeverRegisteredSymbolCountsAsOrphan(t *testing.T) {
	p := newDispatchPool()

	p.routeEvent(m1Event("ZZZZ"))

	if got := atomic.LoadInt64(&p.orphanEvents); got != 1 {
		t.Fatalf("orphanEvents = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&p.lateEvents); got != 0 {
		t.Fatalf("lateEvents = %d, want 0", got)
	}
}

func TestStaleUnregisterDoesNotMarkRetired(t *testing.T) {
	p := newDispatchPool()
	oldID := p.registerDispatch("SHPH", domain.M1, "history-batch", func(rawCandleEvent) {})
	var delivered int
	p.registerDispatch("SHPH", domain.M1, "live", func(rawCandleEvent) { delivered++ })

	p.unregisterDispatchIfCurrent("SHPH", domain.M1, oldID, "history-batch")
	p.routeEvent(m1Event("SHPH"))

	if delivered != 1 {
		t.Fatalf("delivered = %d, want 1", delivered)
	}
	if got := atomic.LoadInt64(&p.lateEvents); got != 0 {
		t.Fatalf("lateEvents = %d, want 0", got)
	}
}

func TestRetiredMarkExpiresAfterTTL(t *testing.T) {
	var r retiredDispatch
	now := time.Now()
	r.mark("SHPH|M1", now)

	if !r.wasRecentlyRetired("SHPH|M1", now.Add(retiredDispatchTTL-time.Second)) {
		t.Fatal("expected mark to be active before TTL")
	}
	if r.wasRecentlyRetired("SHPH|M1", now.Add(retiredDispatchTTL+time.Second)) {
		t.Fatal("expected mark to expire after TTL")
	}
}

func TestRetiredMarksStayBounded(t *testing.T) {
	var r retiredDispatch
	now := time.Now()
	for i := 0; i < retiredDispatchCap+100; i++ {
		r.mark(candleKey(string(rune('A'+i%26))+time.Duration(i).String(), domain.M1), now)
	}
	if len(r.retiredAt) > retiredDispatchCap {
		t.Fatalf("retiredAt = %d, want <= %d", len(r.retiredAt), retiredDispatchCap)
	}
}

func TestRetiredMarksPurgeExpiredBeforeRefusing(t *testing.T) {
	var r retiredDispatch
	old := time.Now()
	for i := 0; i < retiredDispatchCap; i++ {
		r.mark(candleKey(time.Duration(i).String(), domain.M1), old)
	}
	later := old.Add(retiredDispatchTTL + time.Second)

	r.mark("FRESH|M1", later)

	if !r.wasRecentlyRetired("FRESH|M1", later) {
		t.Fatal("expected fresh mark to be accepted after expired ones are purged")
	}
}
