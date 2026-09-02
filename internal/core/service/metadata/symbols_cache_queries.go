package metadata

import (
	"context"
	"fmt"
	"sort"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// Tracked devuelve una copia del universo activo completo -- mismo
// contrato que SymbolRepository.Tracked, servido desde memoria.
func (c *SymbolsCache) Tracked(_ context.Context) ([]domain.Symbol, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sorted, nil
}

// Markets devuelve los mercados distintos con al menos un simbolo activo --
// mismo contrato que SymbolRepository.Markets (ordenado, sin repetidos).
func (c *SymbolsCache) Markets(_ context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	seen := make(map[string]struct{})
	markets := make([]string, 0, 8)
	for _, s := range c.sorted {
		if _, ok := seen[s.Market]; !ok {
			seen[s.Market] = struct{}{}
			markets = append(markets, s.Market)
		}
	}
	sort.Strings(markets)
	return markets, nil
}

// GetBySymbol busca por simbolo exacto -- mismo mensaje de error que
// SymbolRepository.GetBySymbol para no cambiar el contrato de "no existe"
// (el handler HTTP lo mapea a 404).
func (c *SymbolsCache) GetBySymbol(_ context.Context, symbol string) (domain.Symbol, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if s, ok := c.bySymbol[symbol]; ok {
		return s, nil
	}
	return domain.Symbol{}, fmt.Errorf("symbol %s not tracked", symbol)
}
