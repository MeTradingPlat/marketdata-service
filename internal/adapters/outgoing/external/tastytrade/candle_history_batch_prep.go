package tastytrade

import (
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (p *CandlePool) batchTargetSymbols(tf domain.Timeframe, froms map[string]time.Time) []string {
	symbols := make([]string, 0, len(froms))
	for symbol := range froms {
		if tf == domain.M1 && p.LiveSubscribed(symbol) {
			continue
		}
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return symbols
}

// Mismo lock por clave que FetchHistory, en orden lexicografico para
// no interbloquearse con un FetchHistory por-simbolo concurrente.
func (p *CandlePool) lockHistorySymbols(symbols []string, tf domain.Timeframe) func() {
	for _, symbol := range symbols {
		lockVal, _ := p.historyLocks.LoadOrStore(candleKey(symbol, tf), &sync.Mutex{})
		lockVal.(*sync.Mutex).Lock()
	}
	return func() {
		for _, symbol := range symbols {
			if lockVal, ok := p.historyLocks.Load(candleKey(symbol, tf)); ok {
				lockVal.(*sync.Mutex).Unlock()
			}
		}
	}
}

func candleKeys(symbols []string, tf domain.Timeframe) []string {
	keys := make([]string, len(symbols))
	for i, symbol := range symbols {
		keys[i] = candleKey(symbol, tf)
	}
	return keys
}

func (p *CandlePool) unregisterBatchDispatch(active []string, tf domain.Timeframe, dispatchIDs map[string]uint64) {
	for _, symbol := range active {
		p.unregisterDispatchIfCurrent(symbol, tf, dispatchIDs[symbol], "history-batch")
	}
}

func (p *CandlePool) registerBatchDispatch(symbols []string, tf domain.Timeframe, collector *batchHistoryCollector) ([]string, map[string]uint64) {
	dispatchIDs := make(map[string]uint64, len(symbols))
	active := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		sym := symbol
		id := p.registerDispatch(symbol, tf, "history-batch", func(ev rawCandleEvent) { collector.onCandle(sym, ev) })
		if id == 0 {
			continue
		}
		dispatchIDs[symbol] = id
		active = append(active, symbol)
	}
	return active, dispatchIDs
}
