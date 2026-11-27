package tastytrade

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFetchHistoryBatch_AllocateFailureLeavesNoDispatchOrLocks(t *testing.T) {
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		return nil, errors.New("boom")
	}, p.wireChannel, p.handleConnectionReconnect, 1)

	froms := map[string]time.Time{"AAPL": time.Now().Add(-time.Hour)}
	if _, err := p.FetchHistoryBatch(context.Background(), domain.D1, froms); err == nil {
		t.Fatal("expected FetchHistoryBatch to fail")
	}

	p.dispatchMu.RLock()
	_, hasDispatch := p.dispatch[candleKey("AAPL", domain.D1)]
	p.dispatchMu.RUnlock()
	if hasDispatch {
		t.Fatal("expected no dispatch entry after allocate failure")
	}

	if lockVal, ok := p.historyLocks.Load(candleKey("AAPL", domain.D1)); ok {
		if !lockVal.(*sync.Mutex).TryLock() {
			t.Fatal("history lock was not released")
		}
		lockVal.(*sync.Mutex).Unlock()
	}

	if _, err := p.FetchHistoryBatch(context.Background(), domain.D1, froms); err == nil {
		t.Fatal("expected second FetchHistoryBatch to fail without blocking")
	}
}
