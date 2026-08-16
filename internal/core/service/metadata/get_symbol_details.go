package metadata

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getSymbolDetailsService struct {
	symbols      out.SymbolRepository
	fundamentals out.FundamentalsRepository
	intraday     in.GetIntradaySnapshotService
}

func NewGetSymbolDetailsService(symbols out.SymbolRepository, fundamentals out.FundamentalsRepository, intraday in.GetIntradaySnapshotService) in.GetSymbolDetailsService {
	return &getSymbolDetailsService{symbols: symbols, fundamentals: fundamentals, intraday: intraday}
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

	return dto.SymbolDetails{
		ActiveEquity: equity,
		FundamentalData: dto.FundamentalData{
			Symbol:            symbol,
			IsEtf:             fundamentals.IsEtf,
			SecurityType:      domain.ClassifySecurityType(symbol, equity.IsEtf, equity.Description),
			DividendAmount:    fundamentals.DividendAmount,
			DividendFrequency: fundamentals.DividendFrequency,
			DayVolume:         snapshot.DayVolume,
			Open:              snapshot.Open,
			High:              snapshot.High,
			Low:               snapshot.Low,
			PrevClose:         snapshot.PrevClose,
			PreMarketVolume:   snapshot.PreMarketVolume,
			PostMarketVolume:  snapshot.PostMarketVolume,

			TradingStatus: fundamentals.TradingStatus,

			MarketCap:                   fundamentals.MarketCap,
			Eps:                         fundamentals.Eps,
			Beta:                        fundamentals.Beta,
			Lendability:                 fundamentals.Lendability,
			BorrowRate:                  fundamentals.BorrowRate,
			Liquidity:                   fundamentals.Liquidity,
			LiquidityRating:             fundamentals.LiquidityRating,
			ImpliedVolatilityIndex:      fundamentals.ImpliedVolatilityIndex,
			ImpliedVolatilityRank:       fundamentals.ImpliedVolatilityRank,
			ImpliedVolatilityPercentile: fundamentals.ImpliedVolatilityPercentile,
			NextEarningsDate:            fundamentals.NextEarningsDate,
			DaysUntilEarnings:           daysUntil(fundamentals.NextEarningsDate),
		},
	}, nil
}

// daysUntil calcula dias restantes al vuelo en vez de guardarlo -- guardado
// se volveria stale al dia siguiente sin un refresco que lo recalcule.
func daysUntil(isoDate string) int {
	if isoDate == "" {
		return 0
	}
	target, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return 0
	}
	days := int(time.Until(target).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}
