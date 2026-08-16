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
		mergeFormingCandle(&snap, current)
	} else if lastM1, err := s.repo.GetCandles(ctx, symbol, domain.M1, 1, nil); err == nil && len(lastM1) > 0 {
		snap.CurrentPrice = lastM1[0].Close
		snap.CurrentVolume = lastM1[0].Volume
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

// mergeFormingCandle suma la vela M1 en formacion (todavia no guardada, la
// unica fuente que llega hasta el ultimo tick real) a la sesion que le
// corresponde ahora -- sin esto, high/low/volumen del dia quedan hasta un
// minuto atras del precio actual, que si la incluye.
func mergeFormingCandle(snap *domain.IntradaySnapshot, current domain.Candle) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return
	}

	nowET := time.Now().In(loc)
	currentET := current.Timestamp.In(loc)
	// p.current se vacia por completo en cada frontera del barrido nocturno,
	// pero justo despues de medianoche (ET) puede seguir viva la ultima vela
	// de AYER hasta que llegue el primer tick real de hoy -- sin este chequeo
	// esa vela se clasificaria igual (post-market de ayer) pero mezclada
	// dentro del snapshot de HOY, que la query SQL ya calculo vacio.
	if currentET.Year() != nowET.Year() || currentET.YearDay() != nowET.YearDay() {
		return
	}

	marketOpen := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
	marketClose := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 16, 0, 0, 0, loc)

	switch {
	case current.Timestamp.Before(marketOpen):
		snap.PreMarketVolume += current.Volume
		snap.PreMarketClose = current.Close
	case current.Timestamp.Before(marketClose):
		if snap.Open == 0 {
			snap.Open = current.Open
		}
		if snap.High == 0 || current.High > snap.High {
			snap.High = current.High
		}
		if snap.Low == 0 || current.Low < snap.Low {
			snap.Low = current.Low
		}
		snap.DayVolume += current.Volume
	default:
		snap.PostMarketVolume += current.Volume
		snap.PostMarketClose = current.Close
	}
}
