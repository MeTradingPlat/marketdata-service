package fundamentals

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// MergePrevClose -- ver UpsertPrevCloseBatch: closes trae el valor real,
// attemptedOnly son simbolos sin dato (warrant sin M1, etc.) que igual
// avanzan PrevCloseUpdatedAt para no repetir la busqueda esta ventana (ver
// el comentario de domain.Fundamentals.PrevCloseUpdatedAt).
func (c *FundamentalsCache) MergePrevClose(closes map[string]float64, attemptedOnly []string) {
	now := time.Now()
	for symbol, value := range closes {
		v := value
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevClose = &v
			f.PrevCloseUpdatedAt = &now
		})
	}
	for _, symbol := range attemptedOnly {
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevCloseUpdatedAt = &now
		})
	}
}

// MergePrevPostMarketVolume -- mismo criterio que MergePrevClose, ver
// UpsertPrevPostMarketVolumeBatch.
func (c *FundamentalsCache) MergePrevPostMarketVolume(volumes map[string]int64, attemptedOnly []string) {
	now := time.Now()
	for symbol, value := range volumes {
		v := value
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevPostMarketVolume = &v
			f.PrevPostMarketVolumeUpdatedAt = &now
		})
	}
	for _, symbol := range attemptedOnly {
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevPostMarketVolumeUpdatedAt = &now
		})
	}
}

// MergeOpenInterest -- ver UpsertOpenInterestBatch: guarda open_interest y
// open_interest_updated_at en cache.
func (c *FundamentalsCache) MergeOpenInterest(openInterests map[string]float64, attemptedOnly []string) {
	now := time.Now()
	for symbol, value := range openInterests {
		v := value
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.OpenInterest = &v
			f.OpenInterestUpdatedAt = &now
		})
	}
	for _, symbol := range attemptedOnly {
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.OpenInterestUpdatedAt = &now
		})
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
