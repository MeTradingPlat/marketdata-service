package fundamentals

import (
	"strconv"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

// toRealtime arma el punto por punto -- cada bloque solo llena sus campos
// si esa fuente en particular alguna vez corrio para el simbolo (los *At
// nil de domain.Fundamentals), para no confundir "nunca lo buscamos" con
// "lo buscamos y de verdad es cero" (ver dto.FundamentalRealtime).
func toRealtime(symbol string, equity domain.Symbol, f domain.Fundamentals, snapshot domain.IntradaySnapshot) dto.FundamentalRealtime {
	out := dto.FundamentalRealtime{Symbol: symbol}

	if equity.Symbol != "" {
		isEtf := equity.IsEtf
		out.IsEtf = &isEtf
		securityType := domain.ClassifySecurityType(symbol, equity.IsEtf, equity.Description)
		out.SecurityType = &securityType
	}

	applyIntraday(&out, snapshot)

	if f.MarketDataUpdatedAt != nil {
		applyDividendAndHalt(&out, f)
	}
	if f.MetricsUpdatedAt != nil {
		applyMarketMetrics(&out, f, snapshot)
	}
	if f.ExternalUpdatedAt != nil {
		out.SharesOutstanding = f.SharesOutstanding
		out.FloatShares = f.FloatShares
		// El % se calcula al leer contra el float VIGENTE (ver
		// domain.ShortInterestPercent): el float puede corregirse despues
		// de guardarse el dato crudo de FINRA, y un % persistido quedaria
		// desincronizado. Fallback al valor legacy almacenado mientras el
		// proximo refresco de FINRA no haya escrito sharesShorted crudo.
		if computed := domain.ShortInterestPercent(f.ShortInterestShares, f.FloatShares); computed != nil {
			out.ShortInterest = computed
		} else {
			out.ShortInterest = f.ShortInterest
		}
		out.ShortRatio = f.ShortRatio
	}

	return out
}

// applyIntraday siempre corre (nunca hay un "nunca se calculo": GetSnapshot
// no falla salvo error de BD, y en ese caso llega con el zero-value) --
// dayVolume/preMarketVolume/postMarketVolume en 0 es un dato real (sin
// sesion hoy, ej. fin de semana), no ausencia de calculo.
func applyIntraday(out *dto.FundamentalRealtime, snap domain.IntradaySnapshot) {
	dayVolume := snap.DayVolume
	out.DayVolume = &dayVolume
	preVol := snap.PreMarketVolume
	out.PreMarketVolume = &preVol
	postVol := snap.PostMarketVolume
	out.PostMarketVolume = &postVol

	if snap.PrevClose != 0 {
		v := snap.PrevClose
		out.PrevClose = &v
	}
	if snap.Open != 0 {
		v := snap.Open
		out.Open = &v
	}
	if snap.High != 0 {
		v := snap.High
		out.High = &v
	}
	if snap.Low != 0 {
		v := snap.Low
		out.Low = &v
	}
	if snap.PreMarketClose != 0 {
		v := snap.PreMarketClose
		out.PreMarketClose = &v
	}
	if snap.PostMarketClose != 0 {
		v := snap.PostMarketClose
		out.PostMarketClose = &v
	}
}

func applyDividendAndHalt(out *dto.FundamentalRealtime, f domain.Fundamentals) {
	amount := f.DividendAmount
	out.DividendAmount = &amount
	freq := strconv.FormatFloat(f.DividendFrequency, 'f', -1, 64)
	out.DividendFrequency = &freq
	status := f.TradingStatus
	out.TradingStatus = &status
	if f.HaltStartTime >= 0 {
		out.HaltStartTime = &f.HaltStartTime
	}
	if f.HaltEndTime >= 0 {
		out.HaltEndTime = &f.HaltEndTime
	}
}

func applyMarketMetrics(out *dto.FundamentalRealtime, f domain.Fundamentals, snap domain.IntradaySnapshot) {
	marketCap := domain.MarketCapLive(f.MarketCap, f.SharesOutstanding, snap)
	out.MarketCap = &marketCap
	eps := f.Eps
	out.Eps = &eps
	beta := f.Beta
	out.Beta = &beta
	liquidity := f.Liquidity
	out.Liquidity = &liquidity
	liquidityRating := f.LiquidityRating
	out.LiquidityRating = &liquidityRating
	ivIndex := f.ImpliedVolatilityIndex
	out.ImpliedVolatilityIndex = &ivIndex
	ivRank := f.ImpliedVolatilityRank
	out.ImpliedVolatilityRank = &ivRank
	ivPercentile := f.ImpliedVolatilityPercentile
	out.ImpliedVolatilityPercentile = &ivPercentile
	if f.NextEarningsDate != "" {
		days := domain.DaysUntil(f.NextEarningsDate)
		out.DaysUntilEarnings = &days
	}
}
