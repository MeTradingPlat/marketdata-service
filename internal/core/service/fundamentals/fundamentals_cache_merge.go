package fundamentals

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// MergeDividends -- ver UpsertDividends, mismo set de columnas, sin
// COALESCE: la respuesta de /market-data/by-type siempre pisa entero.
func (c *FundamentalsCache) MergeDividends(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			f.DividendAmount = u.DividendAmount
			f.DividendFrequency = u.DividendFrequency
			f.TradingStatus = u.TradingStatus
			f.StatusReason = u.StatusReason
			f.HaltStartTime = u.HaltStartTime
			f.HaltEndTime = u.HaltEndTime
			f.MarketDataUpdatedAt = &now
		})
	}
}

// MergeMarketMetrics -- ver UpsertMarketMetrics, sin COALESCE.
func (c *FundamentalsCache) MergeMarketMetrics(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			f.MarketCap = u.MarketCap
			f.Eps = u.Eps
			f.Beta = u.Beta
			f.Lendability = u.Lendability
			f.BorrowRate = u.BorrowRate
			f.Liquidity = u.Liquidity
			f.LiquidityRating = u.LiquidityRating
			f.ImpliedVolatilityIndex = u.ImpliedVolatilityIndex
			f.ImpliedVolatilityRank = u.ImpliedVolatilityRank
			f.ImpliedVolatilityPercentile = u.ImpliedVolatilityPercentile
			f.NextEarningsDate = u.NextEarningsDate
			f.MetricsUpdatedAt = &now
		})
	}
}

// MergeBeta -- ver UpsertBeta: solo toca beta.
func (c *FundamentalsCache) MergeBeta(updates []domain.Fundamentals) {
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			f.Beta = u.Beta
		})
	}
}

// MergeEarningsHistory -- ver UpsertEarningsHistory: NULLIF+COALESCE en SQL
// (una fecha vacia no pisa una vigente), mismo criterio aca con "" en vez
// de NULL (ver el comentario de domain.Fundamentals.OccurredDate sobre por
// que "" siempre significa "sin dato").
func (c *FundamentalsCache) MergeEarningsHistory(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			if u.OccurredDate != "" {
				f.OccurredDate = u.OccurredDate
			}
			if u.NextEarningsDate != "" {
				f.NextEarningsDate = u.NextEarningsDate
			}
			f.EarningsUpdatedAt = &now
		})
	}
}

// MergeExternalFundamentals -- ver UpsertExternalFundamentals: COALESCE en
// TODOS los campos puntero de SQL, mismo criterio aca (nil no pisa lo que ya
// habia). ShortInterestSettlement es string, no puntero, igual que en la
// fila de Postgres (nunca es NULL de verdad) -- siempre se pisa.
func (c *FundamentalsCache) MergeExternalFundamentals(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			if u.SharesOutstanding != nil {
				f.SharesOutstanding = u.SharesOutstanding
			}
			if u.FloatShares != nil {
				f.FloatShares = u.FloatShares
			}
			if u.ShortInterest != nil {
				f.ShortInterest = u.ShortInterest
			}
			if u.ShortRatio != nil {
				f.ShortRatio = u.ShortRatio
			}
			if u.ShortInterestShares != nil {
				f.ShortInterestShares = u.ShortInterestShares
			}
			f.ShortInterestSettlement = u.ShortInterestSettlement
			if u.InsiderShares != nil {
				f.InsiderShares = u.InsiderShares
			}
			if u.InsiderCiks != nil {
				f.InsiderCiks = u.InsiderCiks
			}
			if u.FloatUpdatedAt != nil {
				f.FloatUpdatedAt = u.FloatUpdatedAt
			}
			f.ExternalUpdatedAt = &now
		})
	}
}
