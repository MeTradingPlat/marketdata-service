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
// empieza a operar sube al instante sin esperar al proximo ReloadAll, sin
// importar cuanto haya operado ayer cualquier otro simbolo.
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
	// hasToday distingue "confirmado con actividad real hoy" de "todavia
	// sin nada hoy, mostrando last_volume solo como referencia" -- sin esta
	// distincion, comparar los dos volumenes como si fueran la misma unidad
	// favorece al que NO opero hoy: last_volume es el TOTAL de todo el dia
	// de ayer (puede ser cientos de millones en un dia de pump real, ver
	// GPRO/DVLT), mientras que el volumen de hoy recien empieza a acumularse
	// -- confirmado en vivo el 2026-09-08, la madrugada del dia siguiente:
	// OFAL (0 hoy, last_volume ~99M de un dia viejo) le ganaba a TSLL/NVDA
	// (decenas de miles reales, ya operando hoy) solo porque sus miles de
	// hoy son un numero mucho mas chico que los millones de ayer de OFAL.
	hasToday bool
}

// rankByTodayVolume reordena matches (en el lugar) EN DOS NIVELES: primero
// todo simbolo con actividad real confirmada hoy (pre-market + regular +
// post-market sumados, ver SnapshotTracker), ordenado por ese volumen; recien
// despues los que todavia no tienen nada hoy, ordenados por su last_volume de
// ayer como referencia de que probablemente valga la pena mirarlos. Nunca se
// comparan ambos numeros entre si (ver el comentario de hasToday) -- eso es
// lo que antes dejaba a un simbolo con miles de acciones reales HOY detras de
// otro con cero hoy pero un last_volume de ayer en los millones.
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
		todayVolume := snap.PreMarketVolume + snap.DayVolume + snap.PostMarketVolume
		hasToday := todayVolume > 0
		volume := s.LastVolume
		if hasToday {
			volume = todayVolume
		}
		ranked[i] = rankedSymbol{symbol: s, volume: volume, hasToday: hasToday}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].hasToday != ranked[j].hasToday {
			return ranked[i].hasToday
		}
		if ranked[i].volume != ranked[j].volume {
			return ranked[i].volume > ranked[j].volume
		}
		return ranked[i].symbol.Symbol < ranked[j].symbol.Symbol
	})
	for i, r := range ranked {
		matches[i] = r.symbol
	}
}
