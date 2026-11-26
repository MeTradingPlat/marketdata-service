package ingestion

import (
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/alerts"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// saveRetryBufferLimit acota la memoria si Postgres tarda en volver -- pasado
// esto se descartan velas nuevas en vez de crecer sin limite (un apagon tan
// largo ya se resuelve solo en la siguiente ventana de mantenimiento, via
// resuscripcion desde watermark).
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
	now         func() time.Time
	notify      func(level, message string, metadata map[string]any)
}

func newSaveRetryBuffer() *saveRetryBuffer {
	return &saveRetryBuffer{now: time.Now, notify: alerts.Notify}
}

func (b *saveRetryBuffer) add(c domain.Candle) {
	b.requeue([]domain.Candle{c})
}

func (b *saveRetryBuffer) requeue(candles []domain.Candle) {
	b.mu.Lock()
	room := max(0, saveRetryBufferLimit-len(b.pending))
	accepted := min(room, len(candles))
	b.pending = append(b.pending, candles[:accepted]...)
	dropped := candles[accepted:]
	if len(dropped) == 0 {
		b.mu.Unlock()
		return
	}
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
