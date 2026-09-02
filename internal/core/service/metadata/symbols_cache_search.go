package metadata

import (
	"context"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// Search reproduce el filtro/orden/paginado de SymbolRepository.Search en
// memoria -- mismo contrato: symbol/description contiene la query (sin
// distinguir mayusculas, equivalente al ILIKE original), market exacto sin
// distinguir mayusculas, orden ya aplicado en ReloadAll (last_volume DESC,
// symbol ASC).
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
