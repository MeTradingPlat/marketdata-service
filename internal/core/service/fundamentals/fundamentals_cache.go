package fundamentals

import (
	"context"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/rs/zerolog/log"
)

// FundamentalsCache mantiene los fundamentales de todo el universo EN
// MEMORIA. Salvo el trading status/HALT (el unico dato que pierde sentido
// si se entera al dia siguiente, ver cmd/api/trading_status_loop.go), los
// fundamentales solo cambian una vez por ventana de mantenimiento -- releer
// Postgres en cada /marketdata/fundamentals/realtime (varios escaneres
// pidiendo el universo completo cada ~60-90s) es trabajo repetido sobre un
// dato que no cambio. ReloadAll() se llama al terminar cada refresh
// (nocturno y el de trading status cada 15 min) -- mismo patron
// write-through que SnapshotTracker ya usa para precios en vivo.
type FundamentalsCache struct {
	repo        out.FundamentalsRepository
	symbols     out.SymbolRepository
	broadcaster *livecandles.Broadcaster[domain.Fundamentals]

	mu   sync.RWMutex
	data map[string]domain.Fundamentals
}

func NewFundamentalsCache(repo out.FundamentalsRepository, symbols out.SymbolRepository, broadcaster *livecandles.Broadcaster[domain.Fundamentals]) *FundamentalsCache {
	return &FundamentalsCache{repo: repo, symbols: symbols, broadcaster: broadcaster, data: make(map[string]domain.Fundamentals)}
}

// ReloadAll relee TODO el universo tracked de una sola consulta y reemplaza
// el cache entero -- mas simple e igual de correcto que parchear campo por
// campo en cada uno de los ~6 puntos de escritura (beta/market metrics/
// earnings/dividends/external/prevClose/trading status), y el costo real es
// bajo (~1s para el universo completo, confirmado en vivo). Un error deja
// el cache anterior intacto en vez de vaciarlo.
func (c *FundamentalsCache) ReloadAll(ctx context.Context) {
	tracked, err := c.symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("fundamentals cache reload: fetching tracked symbols failed, keeping previous data")
		return
	}
	names := make([]string, len(tracked))
	for i, s := range tracked {
		names[i] = s.Symbol
	}
	data, err := c.repo.GetBatch(ctx, names)
	if err != nil {
		log.Error().Err(err).Msg("fundamentals cache reload failed, keeping previous data")
		return
	}
	c.mu.Lock()
	c.data = data
	c.mu.Unlock()

	c.publishAll(data)
}

// publishAll manda cada fundamental recien recargado a quien este
// suscripto a /ws/fundamentals -- ReloadAll solo corre al terminar un
// refresco real (nocturno o el de trading status cada 15 min, ver
// cmd/api/trading_status_loop.go), asi que esto ya es la cadencia "solo
// cuando cambia" que necesita este dato, sin tener que diffear campo por
// campo contra la version anterior.
func (c *FundamentalsCache) publishAll(data map[string]domain.Fundamentals) {
	if c.broadcaster == nil {
		return
	}
	for symbol, f := range data {
		c.broadcaster.Publish(symbol, f)
	}
}

// GetBatch sirve del cache en memoria -- un simbolo sin fundamentales
// conocidos simplemente no aparece, mismo contrato que FundamentalsRepository.GetBatch.
func (c *FundamentalsCache) GetBatch(symbols []string) map[string]domain.Fundamentals {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]domain.Fundamentals, len(symbols))
	for _, s := range symbols {
		if f, ok := c.data[s]; ok {
			result[s] = f
		}
	}
	return result
}
