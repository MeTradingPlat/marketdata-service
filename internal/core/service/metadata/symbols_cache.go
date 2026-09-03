package metadata

import (
	"context"
	"sort"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/rs/zerolog/log"
)

// SymbolsCache mantiene tracked_symbols ENTERO en memoria -- la tabla es
// chica (~13k filas) y solo cambia cuando corre reconcileUniverse (una vez
// al dia) o un Upsert/Deactivate puntual, asi que releerla en cada
// busqueda/listado del frontend (el camino de mas trafico del servicio,
// confirmado en vivo el 2026-09-02) es trabajo repetido sobre un dato casi
// estatico -- mismo principio que FundamentalsCache. ReloadAll() se llama
// al reconciliar el universo, al arranque de cada ciclo (ver
// universe_cycle.go).
type SymbolsCache struct {
	repo    out.SymbolRepository
	tracker *intraday.SnapshotTracker

	mu       sync.RWMutex
	sorted   []domain.Symbol
	bySymbol map[string]domain.Symbol
}

func NewSymbolsCache(repo out.SymbolRepository, tracker *intraday.SnapshotTracker) *SymbolsCache {
	return &SymbolsCache{repo: repo, tracker: tracker, bySymbol: make(map[string]domain.Symbol)}
}

// ReloadAll relee TODO el universo de una sola consulta y reemplaza el
// cache entero -- un error deja el cache anterior intacto en vez de
// vaciarlo (mismo criterio que FundamentalsCache.ReloadAll). Se ordena UNA
// vez aca (last_volume DESC, symbol ASC, mismo orden que Search() en SQL)
// para que Search() solo filtre y pagine, sin reordenar en cada pedido.
func (c *SymbolsCache) ReloadAll(ctx context.Context) {
	symbols, err := c.repo.TrackedWithVolume(ctx)
	if err != nil {
		log.Error().Err(err).Msg("symbols cache reload failed, keeping previous data")
		return
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].LastVolume != symbols[j].LastVolume {
			return symbols[i].LastVolume > symbols[j].LastVolume
		}
		return symbols[i].Symbol < symbols[j].Symbol
	})
	bySymbol := make(map[string]domain.Symbol, len(symbols))
	for _, s := range symbols {
		bySymbol[s.Symbol] = s
	}

	c.mu.Lock()
	c.sorted = symbols
	c.bySymbol = bySymbol
	c.mu.Unlock()
}
