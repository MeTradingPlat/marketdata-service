package tastytrade

import (
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// batchHistoryCollector junta la rafaga historica de MUCHOS simbolos de
// una sola suscripcion -- mismo criterio de quietud que historyCollector
// (la rafaga puede venir en varios FEED_DATA, no hay ACK de fin), pero a
// nivel de lote: silencio de historyQuietPeriod en el lote entero = todos
// los simbolos terminaron de llegar.
type batchHistoryCollector struct {
	tf domain.Timeframe

	mu         sync.Mutex
	data       map[string]map[int64]domain.Candle
	lastUpdate time.Time
}

func newBatchHistoryCollector(tf domain.Timeframe) *batchHistoryCollector {
	return &batchHistoryCollector{tf: tf, data: make(map[string]map[int64]domain.Candle)}
}

func (h *batchHistoryCollector) onCandle(symbol string, ev rawCandleEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	byTs, ok := h.data[symbol]
	if !ok {
		byTs = make(map[int64]domain.Candle)
		h.data[symbol] = byTs
	}
	byTs[ev.Timestamp.UnixMilli()] = mergeCandle(byTs[ev.Timestamp.UnixMilli()], ev, symbol, h.tf)
	h.lastUpdate = time.Now()
}

func (h *batchHistoryCollector) settled() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lastUpdate.IsZero() {
		return false
	}
	hasComplete := false
	for _, byTs := range h.data {
		for _, c := range byTs {
			if c.IsComplete() {
				hasComplete = true
				break
			}
		}
		if hasComplete {
			break
		}
	}
	return hasComplete && time.Since(h.lastUpdate) >= historyQuietPeriod
}

func (h *batchHistoryCollector) complete() map[string][]domain.Candle {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make(map[string][]domain.Candle, len(h.data))
	for symbol, byTs := range h.data {
		candles := make([]domain.Candle, 0, len(byTs))
		for _, c := range byTs {
			if c.IsComplete() {
				candles = append(candles, c)
			}
		}
		sort.Slice(candles, func(i, j int) bool { return candles[i].Timestamp.Before(candles[j].Timestamp) })
		result[symbol] = candles
	}
	return result
}
