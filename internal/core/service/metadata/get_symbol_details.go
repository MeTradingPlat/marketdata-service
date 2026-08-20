package metadata

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getSymbolDetailsService struct {
	symbols      out.SymbolRepository
	fundamentals out.FundamentalsRepository
	intraday     in.GetIntradaySnapshotService
	openInterest out.OpenInterestGateway
}

func NewGetSymbolDetailsService(symbols out.SymbolRepository, fundamentals out.FundamentalsRepository, intraday in.GetIntradaySnapshotService, openInterest out.OpenInterestGateway) in.GetSymbolDetailsService {
	return &getSymbolDetailsService{symbols: symbols, fundamentals: fundamentals, intraday: intraday, openInterest: openInterest}
}

func (s *getSymbolDetailsService) GetSymbolDetails(ctx context.Context, symbol string) (dto.SymbolDetails, error) {
	equity, err := s.symbols.GetBySymbol(ctx, symbol)
	if err != nil {
		return dto.SymbolDetails{}, err
	}

	fundamentals, err := s.fundamentals.Get(ctx, symbol)
	if err != nil {
		return dto.SymbolDetails{}, err
	}

	// El snapshot intradia (OHLC/volumen del dia) es una fuente separada de
	// los dividendos -- si falla (ej. sin velas D1 todavia para un simbolo
	// nuevo), se sirve igual el resto en vez de tirar todo el detalle.
	snapshot, err := s.intraday.GetSnapshot(ctx, symbol)
	if err != nil {
		snapshot = domain.IntradaySnapshot{}
	}

	details := dto.SymbolDetails{
		ActiveEquity: equity,
		FundamentalData: dto.FundamentalData{
			Symbol:            symbol,
			IsEtf:             fundamentals.IsEtf,
			SecurityType:      domain.ClassifySecurityType(symbol, equity.IsEtf, equity.Description),
			DividendAmount:    fundamentals.DividendAmount,
			DividendFrequency: fundamentals.DividendFrequency,
			DayVolume:         snapshot.DayVolume,
			Open:              nonzeroOrNil(snapshot.Open),
			High:              nonzeroOrNil(snapshot.High),
			Low:               nonzeroOrNil(snapshot.Low),
			PrevClose:         nonzeroOrNil(snapshot.PrevClose),
			PreMarketVolume:   snapshot.PreMarketVolume,
			PostMarketVolume:  snapshot.PostMarketVolume,

			TradingStatus: fundamentals.TradingStatus,

			MarketCap:                   nonzeroOrNil(domain.MarketCapLive(fundamentals.MarketCap, fundamentals.SharesOutstanding, snapshot)),
			Eps:                         nonzeroOrNil(fundamentals.Eps),
			Beta:                        nonzeroOrNil(fundamentals.Beta),
			Lendability:                 fundamentals.Lendability,
			BorrowRate:                  fundamentals.BorrowRate,
			Liquidity:                   fundamentals.Liquidity,
			LiquidityRating:             fundamentals.LiquidityRating,
			ImpliedVolatilityIndex:      fundamentals.ImpliedVolatilityIndex,
			ImpliedVolatilityRank:       fundamentals.ImpliedVolatilityRank,
			ImpliedVolatilityPercentile: fundamentals.ImpliedVolatilityPercentile,
			NextEarningsDate:            fundamentals.NextEarningsDate,
			DaysUntilEarnings:           domain.DaysUntil(fundamentals.NextEarningsDate),
			OccurredDate:                fundamentals.OccurredDate,
		},
	}
	applyExternalFundamentals(&details.FundamentalData, fundamentals)
	if oi, ok := s.openInterest.OpenInterest(ctx, symbol); ok && oi > 0 {
		details.FundamentalData.OpenInterest = &oi
	}
	return details, nil
}

// nonzeroOrNil es el mismo criterio de "0 no es un dato" que toRealtime()
// en el paquete fundamentals: OHLC/precio en 0 significa que esa sesion
// todavia no empezo, no un precio real -- el campo se omite del JSON y el
// frontend muestra "N/A" en vez de un 0 que parece un dato.
func nonzeroOrNil(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

// applyExternalFundamentals solo llena sharesOutstanding/floatShares/
// shortInterest/shortRatio si ExternalUpdatedAt esta seteado -- misma
// disciplina null-vs-0 que toRealtime() en el paquete fundamentals (ver
// realtime_mapper.go), para no mostrar "0" donde en realidad nunca se
// corrio el refresco de SEC EDGAR/FINRA para este simbolo.
func applyExternalFundamentals(out *dto.FundamentalData, f domain.Fundamentals) {
	if f.ExternalUpdatedAt == nil {
		return
	}
	out.SharesOutstanding = f.SharesOutstanding
	out.FloatShares = f.FloatShares
	// % calculado al leer contra el float vigente -- mismo criterio que
	// toRealtime() (ver realtime_mapper.go).
	if computed := domain.ShortInterestPercent(f.ShortInterestShares, f.FloatShares); computed != nil {
		out.ShortInterest = computed
	} else {
		out.ShortInterest = f.ShortInterest
	}
	out.ShortRatio = f.ShortRatio
	if f.FloatShares != nil {
		source := "ESTIMATED"
		if f.FloatUpdatedAt != nil {
			source = "SEC_EDGAR"
		}
		out.FloatSource = &source
	}
}
