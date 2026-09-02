package metadata

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	fundamentalscache "github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

type getSymbolDetailsService struct {
	symbols      out.SymbolRepository
	fundamentals *fundamentalscache.FundamentalsCache
	intraday     in.GetIntradaySnapshotService
	openInterest out.OpenInterestGateway
}

func NewGetSymbolDetailsService(symbols out.SymbolRepository, fundamentals *fundamentalscache.FundamentalsCache, intraday in.GetIntradaySnapshotService, openInterest out.OpenInterestGateway) in.GetSymbolDetailsService {
	return &getSymbolDetailsService{symbols: symbols, fundamentals: fundamentals, intraday: intraday, openInterest: openInterest}
}

func (s *getSymbolDetailsService) GetSymbolDetails(ctx context.Context, symbol string) (dto.SymbolDetails, error) {
	equity, err := s.symbols.GetBySymbol(ctx, symbol)
	if err != nil {
		return dto.SymbolDetails{}, err
	}

	// Servido desde el cache en memoria (ya se recarga completo tras cada
	// refresh nocturno/de trading status, ver FundamentalsCache) en vez de
	// una consulta a Postgres por cada apertura del panel de detalle --
	// confirmado en vivo el 2026-09-02: este endpoint era el unico camino de
	// fundamentales que todavia pegaba directo a la BD por simbolo, y un
	// simbolo sin fundamentales conocidos (recien trackeado) volvia error
	// duro en vez de mostrar igual el resto del detalle (mismo criterio de
	// "no tirar todo por una parte que falta" que ya se usa abajo con el
	// snapshot intradia).
	fundamentals := s.fundamentals.GetBatch([]string{symbol})[symbol]

	// El snapshot intradia (OHLC/volumen del dia) es una fuente separada de
	// los dividendos -- si falla (ej. sin velas D1 todavia para un simbolo
	// nuevo), se sirve igual el resto en vez de tirar todo el detalle.
	snapshot, err := s.intraday.GetSnapshot(ctx, symbol)
	if err != nil {
		snapshot = domain.IntradaySnapshot{}
	}

	// Una next_earnings_date que ya paso no es "la proxima" de nada (ver
	// domain.IsFutureOrToday) -- tratarla como si nunca se hubiera buscado
	// evita mostrar una fecha de hace semanas como si fuera la que viene.
	nextEarnings := fundamentals.NextEarningsDate
	if !domain.IsFutureOrToday(nextEarnings) {
		nextEarnings = ""
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
			NextEarningsDate:            nextEarnings,
			DaysUntilEarnings:           daysUntilOrNil(nextEarnings),
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

// daysUntilOrNil: domain.DaysUntil("") devuelve 0, indistinguible de "los
// earnings son hoy" -- mismo criterio null-vs-0 que nonzeroOrNil, aplicado
// a la fecha de origen en vez del resultado.
func daysUntilOrNil(isoDate string) *int {
	if isoDate == "" {
		return nil
	}
	days := domain.DaysUntil(isoDate)
	return &days
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
