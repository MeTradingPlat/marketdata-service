package ingestion

import (
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// saveRetryBufferLimit acota la memoria si Postgres tarda en volver -- pasado
// esto se descartan velas nuevas en vez de crecer sin limite (un apagon tan
// largo ya se resuelve solo en la siguiente ventana de mantenimiento, via
// resuscripcion desde watermark).
const saveRetryBufferLimit = 5000

// saveRetryBuffer guarda las velas en vivo que fallaron al guardarse -- la
// suscripcion DxLink en si nunca se entera de un fallo de Postgres (sigue
// sana, sigue mandando datos), asi que lo unico que de verdad se pierde es
// el guardado puntual. Save() es UPSERT, reintentar de mas nunca duplica.
type saveRetryBuffer struct {
	mu      sync.Mutex
	pending []domain.Candle
}

func newSaveRetryBuffer() *saveRetryBuffer {
	return &saveRetryBuffer{}
}

func (b *saveRetryBuffer) add(c domain.Candle) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.pending) >= saveRetryBufferLimit {
		return
	}
	b.pending = append(b.pending, c)
}

func (b *saveRetryBuffer) requeue(candles []domain.Candle) {
	b.mu.Lock()
	defer b.mu.Unlock()
	room := saveRetryBufferLimit - len(b.pending)
	if room <= 0 {
		return
	}
	if room > len(candles) {
		room = len(candles)
	}
	b.pending = append(b.pending, candles[:room]...)
}

func (b *saveRetryBuffer) drain() []domain.Candle {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.pending
	b.pending = nil
	return out
}
