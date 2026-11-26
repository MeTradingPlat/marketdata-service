package metadata

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

// toRealtime arma el punto por punto -- cada bloque solo llena sus campos
// si esa fuente en particular alguna vez corrio para el simbolo (los *At
// nil de domain.Fundamentals), para no confundir "nunca lo buscamos" con
// "lo buscamos y de verdad es cero" (ver dto.FundamentalRealtime).
func toRealtime(symbol string, equity domain.Symbol, f domain.Fundamentals, snapshot domain.IntradaySnapshot) dto.FundamentalRealtime {
	out := dto.FundamentalRealtime{Symbol: symbol}
	applyEquityInfo(&out, symbol, equity)
	applyIntraday(&out, snapshot)
	if f.MarketDataUpdatedAt != nil {
		applyDividendAndHalt(&out, f)
	}
	if f.MetricsUpdatedAt != nil {
		applyMarketMetrics(&out, f, snapshot)
	}
	applyExternalRealtime(&out, f)
	if f.PrevPostMarketVolumeUpdatedAt != nil {
		out.PrevPostMarketVolume = f.PrevPostMarketVolume
	}
	if f.OpenInterest != nil {
		oi := *f.OpenInterest
		out.OpenInterest = &oi
	}
	return out
}

func applyEquityInfo(out *dto.FundamentalRealtime, symbol string, equity domain.Symbol) {
	if equity.Symbol == "" {
		return
	}
	isEtf := equity.IsEtf
	out.IsEtf = &isEtf
	securityType := domain.ClassifySecurityType(symbol, equity.IsEtf, equity.Description)
	out.SecurityType = &securityType
}

func applyExternalRealtime(out *dto.FundamentalRealtime, f domain.Fundamentals) {
	if f.ExternalUpdatedAt == nil {
		return
	}
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
