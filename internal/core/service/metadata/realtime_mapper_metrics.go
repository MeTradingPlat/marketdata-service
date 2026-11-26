package metadata

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

// marketCap/beta/liquidity/IV en 0 no son un dato real (confirmado en
// vivo con MSTU, un ETF apalancado sin market cap ni EPS de TastyTrade) --
// mismo criterio "0 no es un dato" que applyIntraday de arriba, que estos
// campos no tenian: un MARKET_CAP MENOR_QUE en signal-processing-service
// evaluaba $0 como un dato real y disparaba falso positivo en cualquier
// simbolo sin este dato, en vez de excluirlo por falta de dato como
// evaluate() ya hace cuando compute_value devuelve None.
func applyMarketMetrics(out *dto.FundamentalRealtime, f domain.Fundamentals, snap domain.IntradaySnapshot) {
	out.MarketCap = nonzeroOrNil(domain.MarketCapLive(f.MarketCap, f.SharesOutstanding, snap))
	if f.Eps != nil {
		eps := *f.Eps
		out.Eps = &eps
	}
	out.Beta = nonzeroOrNil(f.Beta)
	out.Liquidity = nonzeroOrNil(f.Liquidity)
	liquidityRating := f.LiquidityRating
	out.LiquidityRating = &liquidityRating
	out.ImpliedVolatilityIndex = nonzeroOrNil(f.ImpliedVolatilityIndex)
	out.ImpliedVolatilityRank = nonzeroOrNil(f.ImpliedVolatilityRank)
	out.ImpliedVolatilityPercentile = nonzeroOrNil(f.ImpliedVolatilityPercentile)
	// IsFutureOrToday, no solo != "" -- una next_earnings_date que ya paso
	// (confirmado en vivo con INTC, casi un mes vieja) no es "la proxima" de
	// nada, ver el comentario de domain.IsFutureOrToday.
	if domain.IsFutureOrToday(f.NextEarningsDate) {
		days := domain.DaysUntil(f.NextEarningsDate)
		out.DaysUntilEarnings = &days
		next := f.NextEarningsDate
		out.NextEarningsDate = &next
	}
}
