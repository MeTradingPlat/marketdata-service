package intraday

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getIntradaySnapshotService struct {
	repo    out.CandleRepository
	gateway out.MarketDataGateway
}

func NewGetIntradaySnapshotService(repo out.CandleRepository, gateway out.MarketDataGateway) in.GetIntradaySnapshotService {
	return &getIntradaySnapshotService{repo: repo, gateway: gateway}
}

// GetSnapshot arma todo lo que se puede sacar de fundamentales SOLO con
// nuestras propias velas -- sesiones de hoy (repo), precio/volumen actual
// (la vela M1 en formacion, o la ultima M1 cerrada si el simbolo no esta
// en vivo ahora mismo), y el cierre D1 mas reciente como prevClose.
func (s *getIntradaySnapshotService) GetSnapshot(ctx context.Context, symbol string) (domain.IntradaySnapshot, error) {
	snap, err := s.repo.GetIntradaySessions(ctx, symbol)
	if err != nil {
		return domain.IntradaySnapshot{}, fmt.Errorf("getting intraday sessions for %s: %w", symbol, err)
	}
	snap.Symbol = symbol
	snap.AsOf = time.Now()

	if current, ok := s.gateway.CurrentCandle(symbol); ok {
		snap.CurrentPrice = current.Close
		snap.CurrentVolume = current.Volume
	} else if lastM1, err := s.repo.GetCandles(ctx, symbol, domain.M1, 1); err == nil && len(lastM1) > 0 {
		snap.CurrentPrice = lastM1[0].Close
		snap.CurrentVolume = lastM1[0].Volume
	}

	prevDay, err := s.repo.GetCandles(ctx, symbol, domain.D1, 1)
	if err != nil {
		return domain.IntradaySnapshot{}, fmt.Errorf("getting previous close for %s: %w", symbol, err)
	}
	if len(prevDay) > 0 {
		snap.PrevClose = prevDay[0].Close
	}

	return snap, nil
}
