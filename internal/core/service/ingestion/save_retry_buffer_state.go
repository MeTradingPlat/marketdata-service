package ingestion

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (b *saveRetryBuffer) shouldAlertLocked() bool {
	now := b.now()
	if !b.lastAlertAt.IsZero() && now.Sub(b.lastAlertAt) < retryOverflowAlertInterval {
		return false
	}
	b.lastAlertAt = now
	return true
}

func (b *saveRetryBuffer) size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

func (b *saveRetryBuffer) discardedCount() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.discarded
}

func (b *saveRetryBuffer) drain() []domain.Candle {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.pending
	b.pending = nil
	return out
}
