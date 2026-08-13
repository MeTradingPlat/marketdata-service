package tastytrade

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	historyPollInterval = 200 * time.Millisecond
	historyQuietPeriod  = 700 * time.Millisecond
)

type historyCollector struct {
	symbol string
	tf     domain.Timeframe

	mu         sync.Mutex
	data       map[int64]domain.Candle
	lastUpdate time.Time
}

func newHistoryCollector(symbol string, tf domain.Timeframe) *historyCollector {
	return &historyCollector{symbol: symbol, tf: tf, data: make(map[int64]domain.Candle)}
}

func (h *historyCollector) onCandle(ev rawCandleEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := ev.Timestamp.UnixMilli()
	h.data[key] = mergeCandle(h.data[key], ev, h.symbol, h.tf)
	h.lastUpdate = time.Now()
}

func (h *historyCollector) hasData() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.data {
		if c.IsComplete() {
			return true
		}
	}
	return false
}

// settled es verdadero una vez que ya llego al menos una vela Y paso un
// periodo sin recibir nada nuevo -- dxLink puede repartir una rafaga
// historica en varios mensajes FEED_DATA separados en el tiempo (no
// necesariamente uno solo atomico), asi que "ya hay datos" no es lo mismo
// que "ya llego toda la rafaga" (confirmado en vivo: cortar en la primera
// vela completa daba solo 2 filas de D1 en vez de años de historia).
func (h *historyCollector) settled() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lastUpdate.IsZero() {
		return false
	}
	hasComplete := false
	for _, c := range h.data {
		if c.IsComplete() {
			hasComplete = true
			break
		}
	}
	return hasComplete && time.Since(h.lastUpdate) >= historyQuietPeriod
}

func (h *historyCollector) complete() []domain.Candle {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]domain.Candle, 0, len(h.data))
	for _, c := range h.data {
		if c.IsComplete() {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	return result
}

func waitForData(ctx context.Context, settled func() bool, timeout time.Duration) error {
	ticker := time.NewTicker(historyPollInterval)
	defer ticker.Stop()
	deadline := time.After(timeout)
	for {
		if settled() {
			return nil
		}
		select {
		case <-ticker.C:
			continue
		case <-deadline:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
