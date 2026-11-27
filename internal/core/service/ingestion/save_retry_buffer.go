package ingestion

import (
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/alerts"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// saveRetryBufferLimit acota la memoria si Postgres tarda en volver -- pasado
// esto el sobrante pasa al spool en disco y solo se descarta si no hay spool
// o falla al escribir.
const saveRetryBufferLimit = 100000

const retryOverflowAlertInterval = 15 * time.Minute

// saveRetryBuffer guarda las velas en vivo que fallaron al guardarse -- la
// suscripcion DxLink en si nunca se entera de un fallo de Postgres (sigue
// sana, sigue mandando datos), asi que lo unico que de verdad se pierde es
// el guardado puntual. Save() es UPSERT, reintentar de mas nunca duplica.
type saveRetryBuffer struct {
	mu          sync.Mutex
	pending     []domain.Candle
	discarded   int64
	lastAlertAt time.Time
	spool       out.CandleSpool
	now         func() time.Time
	notify      func(level, message string, metadata map[string]any)
}

func newSaveRetryBuffer(spool out.CandleSpool) *saveRetryBuffer {
	return &saveRetryBuffer{spool: spool, now: time.Now, notify: alerts.Notify}
}

func (b *saveRetryBuffer) add(c domain.Candle) {
	b.requeue([]domain.Candle{c})
}

func (b *saveRetryBuffer) requeue(candles []domain.Candle) {
	b.mu.Lock()
	room := max(0, saveRetryBufferLimit-len(b.pending))
	accepted := min(room, len(candles))
	b.pending = append(b.pending, candles[:accepted]...)
	overflow := candles[accepted:]
	spool := b.spool
	b.mu.Unlock()
	if len(overflow) == 0 {
		return
	}
	if spool != nil {
		if err := spool.Write(overflow); err == nil {
			log.Warn().Int("candles", len(overflow)).Msg("save retry buffer overflow, spooled candles to disk")
			return
		} else {
			log.Error().Err(err).Int("candles", len(overflow)).Msg("writing candle overflow to disk spool failed")
		}
	}
	b.recordDiscarded(overflow)
}

func (b *saveRetryBuffer) restore(candles []domain.Candle) {
	if len(candles) == 0 {
		return
	}
	b.mu.Lock()
	b.pending = append(candles, b.pending...)
	b.mu.Unlock()
}

func (b *saveRetryBuffer) recordDiscarded(dropped []domain.Candle) {
	b.mu.Lock()
	b.discarded += int64(len(dropped))
	if !b.shouldAlertLocked() {
		b.mu.Unlock()
		return
	}
	total := b.discarded
	notify := b.notify
	b.mu.Unlock()
	reportRetryOverflow(len(dropped), total, sampleCandleSymbols(dropped, 20), notify)
}
