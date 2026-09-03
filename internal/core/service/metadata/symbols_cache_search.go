package metadata

import (
	"context"
	"sort"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// Search reproduce el filtro de SymbolRepository.Search en memoria: symbol/
// description contiene la query (sin distinguir mayusculas, equivalente al
// ILIKE original), market exacto sin distinguir mayusculas. El orden NO es
// el estatico de ReloadAll -- se recalcula en cada pedido contra la
// actividad de HOY (ver rankByTodayVolume), asi que un simbolo que recien
// empieza a operar sube al instante sin esperar al proximo ReloadAll.
func (c *SymbolsCache) Search(_ context.Context, query string, markets []string, page, size int) ([]domain.Symbol, int64, error) {
	c.mu.RLock()
	sorted := c.sorted
	c.mu.RUnlock()

	allowed := upperSet(markets)
	q := strings.ToUpper(query)

	matches := make([]domain.Symbol, 0, len(sorted))
	for _, s := range sorted {
		if len(allowed) > 0 {
			if _, ok := allowed[strings.ToUpper(s.Market)]; !ok {
				continue
			}
		}
		if q != "" && !strings.Contains(strings.ToUpper(s.Symbol), q) && !strings.Contains(strings.ToUpper(s.Description), q) {
			continue
		}
		matches = append(matches, s)
	}

	c.rankByTodayVolume(matches)

	total := int64(len(matches))
	start := page * size
	if start < 0 || start >= len(matches) {
		return []domain.Symbol{}, total, nil
	}
	end := start + size
	if end > len(matches) {
		end = len(matches)
	}
	return matches[start:end], total, nil
}

func upperSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[strings.ToUpper(v)] = struct{}{}
	}
	return set
}

type rankedSymbol struct {
	symbol domain.Symbol
	volume int64
}

// rankByTodayVolume reordena matches (en el lugar) por actividad REAL de hoy
// -- pre-market + regular + post-market sumados (ver SnapshotTracker), que
// es el volumen total del dia para el simbolo -- en vez del last_volume
// estatico de ayer que trae ReloadAll. Mientras el mercado no abrio, la
// suma de hoy es 0 para todos y el orden cae solo a last_volume (mismo
// resultado que antes); apenas entra volumen real a cualquier sesion, el
// que mas opero hoy pasa a estar primero, sin esperar al proximo ReloadAll
// (que corre una vez al dia).
func (c *SymbolsCache) rankByTodayVolume(matches []domain.Symbol) {
	if c.tracker == nil || len(matches) == 0 {
		return
	}
	symbols := make([]string, len(matches))
	for i, s := range matches {
		symbols[i] = s.Symbol
	}
	today := c.tracker.SnapshotBatch(symbols)

	ranked := make([]rankedSymbol, len(matches))
	for i, s := range matches {
		snap := today[s.Symbol]
		volume := s.LastVolume
		if todayVolume := snap.PreMarketVolume + snap.DayVolume + snap.PostMarketVolume; todayVolume > 0 {
			volume = todayVolume
		}
		ranked[i] = rankedSymbol{symbol: s, volume: volume}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].volume != ranked[j].volume {
			return ranked[i].volume > ranked[j].volume
		}
		return ranked[i].symbol.Symbol < ranked[j].symbol.Symbol
	})
	for i, r := range ranked {
		matches[i] = r.symbol
	}
}
