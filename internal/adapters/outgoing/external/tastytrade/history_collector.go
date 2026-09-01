package tastytrade

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

const (
	historyPollInterval = 200 * time.Millisecond
	historyQuietPeriod  = 700 * time.Millisecond
)

// eventFlagsConfirmedOnce: TastyTrade no siempre puebla todo lo documentado
// por dxFeed (ver profileEventFields) -- esto confirma UNA sola vez por
// proceso, a nivel Info (el resto del logging de este archivo es Debug,
// invisible con el SetGlobalLevel de main.go), si el feed realmente manda
// eventFlags o si el fix de snapshotDone es un no-op silencioso.
var eventFlagsConfirmedOnce sync.Once

type historyCollector struct {
	symbol string
	tf     domain.Timeframe

	mu              sync.Mutex
	data            map[int64]domain.Candle
	lastUpdate      time.Time
	sawSnapshotDone bool
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

	if !h.sawSnapshotDone && ev.snapshotDone() {
		h.sawSnapshotDone = true
		snipped := ev.EventFlags&eventFlagSnapshotSnip != 0
		log.Debug().Str("symbol", h.symbol).Str("timeframe", string(h.tf)).Bool("snipped", snipped).
			Msg("dxlink marked historical snapshot done via eventFlags")
		eventFlagsConfirmedOnce.Do(func() {
			log.Info().Str("symbol", h.symbol).Str("timeframe", string(h.tf)).Bool("snipped", snipped).
				Msg("confirmed: dxlink populates eventFlags for Candle, snapshot completion no longer guessed by timeout alone")
		})
	}
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

// settled es verdadero cuando ya hay al menos una vela Y (a) dxLink marco
// el final real de la rafaga via eventFlags (SNAPSHOT_END/SNAPSHOT_SNIP,
// ver rawCandleEvent.snapshotDone) o, si ese campo no llega poblado para
// este feed, (b) paso un periodo sin recibir nada nuevo -- dxLink puede
// repartir una rafaga historica en varios mensajes FEED_DATA separados en
// el tiempo (no necesariamente uno solo atomico), asi que "ya hay datos"
// no es lo mismo que "ya llego toda la rafaga" (confirmado en vivo: cortar
// en la primera vela completa daba solo 2 filas de D1 en vez de años de
// historia). (b) es el respaldo original -- un timeout de reloj fijo
// (historyDeepWait) puede cortar a mitad de una rafaga todavia activa para
// un simbolo de mucho volumen, confirmado en vivo el 2026-08-31 con
// FXI/PFE/IBIT (profundidad M1 mucho mas corta que el resto del universo,
// cada uno cortado en una fecha distinta). (a) evita ese corte prematuro
// cuando dxFeed si puebla el campo.
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
	if !hasComplete {
		return false
	}
	return h.sawSnapshotDone || time.Since(h.lastUpdate) >= historyQuietPeriod
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
