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
	tracker *SnapshotTracker
}

func NewGetIntradaySnapshotService(repo out.CandleRepository, gateway out.MarketDataGateway, tracker *SnapshotTracker) in.GetIntradaySnapshotService {
	return &getIntradaySnapshotService{repo: repo, gateway: gateway, tracker: tracker}
}

// GetSnapshot arma todo lo que se puede sacar de fundamentales SOLO con
// nuestras propias velas -- sesiones de hoy (tracker en memoria, o repo si no esta),
// precio/volumen actual (la vela M1 en formacion, o la ultima M1 cerrada), y el cierre D1
// mas reciente como prevClose.
func (s *getIntradaySnapshotService) GetSnapshot(ctx context.Context, symbol string) (domain.IntradaySnapshot, error) {
	var snap domain.IntradaySnapshot
	var found bool
	if s.tracker != nil {
		snap, found = s.tracker.Snapshot(symbol)
	}
	if !found {
		var err error
		snap, err = s.repo.GetIntradaySessions(ctx, symbol)
		if err != nil {
			return domain.IntradaySnapshot{}, fmt.Errorf("getting intraday sessions for %s: %w", symbol, err)
		}
	}
	snap.Symbol = symbol
	snap.AsOf = time.Now()

	if current, ok := s.gateway.CurrentCandle(symbol); ok {
		snap.CurrentPrice = current.Close
		snap.CurrentVolume = current.Volume
		mergeFormingCandle(&snap, current)
	} else if price, volume, ok := s.tracker.LastClose(symbol); ok {
		snap.CurrentPrice = price
		snap.CurrentVolume = volume
	} else if lastM1, err := s.repo.GetCandles(ctx, symbol, domain.M1, 1, nil); err == nil && len(lastM1) > 0 {
		snap.CurrentPrice = lastM1[0].Close
		snap.CurrentVolume = lastM1[0].Volume
	}

	if snap.PrevClose > 0 {
		return snap, nil
	}

	if s.tracker != nil {
		if price, _, ok := s.tracker.LastClose(symbol); ok && price > 0 {
			snap.PrevClose = price
			return snap, nil
		}
	}

	// prevClose = cierre REGULAR de la sesion anterior (subasta 16:00 ET)
	// sacado de M1 -- la vela D1 que se guardaba antes cierra a las 20:00 ET
	// con el post-market incluido, que no es el cierre oficial que usa el
	// resto de la industria (y que esperan los calculos de % del dia). Si no
	// hay vela de subasta (media sesion, simbolo sin profundidad M1), cae a
	// la D1 mas reciente -- mejor un cierre extendido que nada.
	loc, err := time.LoadLocation("America/New_York")
	if err == nil {
		nowET := time.Now().In(loc)
		openET := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
		if prev, perr := s.repo.GetPreviousSessionClose(ctx, symbol, openET); perr == nil && prev != nil {
			snap.PrevClose = *prev
			return snap, nil
		}
	}
	prevDay, err := s.repo.GetCandles(ctx, symbol, domain.D1, 1, nil)
	if err != nil {
		return domain.IntradaySnapshot{}, fmt.Errorf("getting previous close for %s: %w", symbol, err)
	}
	if len(prevDay) > 0 {
		snap.PrevClose = prevDay[0].Close
	}

	return snap, nil
}
