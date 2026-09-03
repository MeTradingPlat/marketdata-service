package ingestion

import (
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// liveSaveBuffer junta las velas recien cerradas de TODOS los simbolos
// entre flushes en vez de guardar cada una al instante -- con ~13k simbolos
// cerrando su propia vela de minuto en momentos distintos dentro del mismo
// minuto, guardar una por una significaba hasta ~13k escrituras
// individuales a Postgres por minuto. FlushLiveSaves (llamado cada pocos
// segundos desde cmd/api) las junta en un solo Save() por lote -- Save()
// ya aceptaba []domain.Candle, solo se llamaba con una sola vela adentro.
// Sin tope de tamaño (a diferencia de saveRetryBuffer): el intervalo de
// flush es corto a proposito para que nunca acumule mas que un par de
// segundos de actividad real.
type liveSaveBuffer struct {
	mu      sync.Mutex
	pending []domain.Candle
}

func newLiveSaveBuffer() *liveSaveBuffer {
	return &liveSaveBuffer{}
}

func (b *liveSaveBuffer) add(c domain.Candle) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = append(b.pending, c)
}

func (b *liveSaveBuffer) drain() []domain.Candle {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.pending
	b.pending = nil
	return out
}
