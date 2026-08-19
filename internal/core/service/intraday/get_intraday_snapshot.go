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

// GetSnapshotsBatch es GetSnapshot para un lote de simbolos en un numero fijo
// de queries en vez de 2-3 por simbolo -- confirmado en vivo el 2026-08-19:
// fundamentals/realtime llamaba GetSnapshot simbolo por simbolo (20 workers
// contra el pool de 20 conexiones) y con el universo NYSE+NASDAQ+AMEX
// (~8800 simbolos) eso tardaba ~90.2-90.3s constantes, justo encima del
// timeout de 90s del cliente -- el escaner de pre-mercado perdia esa carrera
// en CADA ciclo y nunca vio un solo simbolo con datos, 0 senales toda la
// sesion pese a filtros poco exigentes.
func (s *getIntradaySnapshotService) GetSnapshotsBatch(ctx context.Context, symbols []string) map[string]domain.IntradaySnapshot {
	result := make(map[string]domain.IntradaySnapshot, len(symbols))
	if len(symbols) == 0 {
		return result
	}

	sessions, err := s.repo.GetIntradaySessionsBatch(ctx, symbols)
	if err != nil {
		sessions = map[string]domain.IntradaySnapshot{}
	}

	now := time.Now()
	needD1Fallback := make([]string, 0)
	var prevCloses map[string]float64
	loc, locErr := time.LoadLocation("America/New_York")
	if locErr == nil {
		nowET := now.In(loc)
		openET := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
		prevCloses, err = s.repo.GetPreviousSessionCloseBatch(ctx, symbols, openET)
		if err != nil {
			prevCloses = map[string]float64{}
		}
	} else {
		prevCloses = map[string]float64{}
	}

	for _, symbol := range symbols {
		snap, ok := sessions[symbol]
		if !ok {
			snap = domain.IntradaySnapshot{Symbol: symbol}
		}
		snap.Symbol = symbol
		snap.AsOf = now

		if current, ok := s.gateway.CurrentCandle(symbol); ok {
			snap.CurrentPrice = current.Close
			snap.CurrentVolume = current.Volume
			mergeFormingCandle(&snap, current)
		} else if lastM1, err := s.repo.GetCandles(ctx, symbol, domain.M1, 1, nil); err == nil && len(lastM1) > 0 {
			snap.CurrentPrice = lastM1[0].Close
			snap.CurrentVolume = lastM1[0].Volume
		}

		if prev, ok := prevCloses[symbol]; ok {
			snap.PrevClose = prev
		} else {
			needD1Fallback = append(needD1Fallback, symbol)
		}

		result[symbol] = snap
	}

	// prevClose D1 de respaldo (subasta ausente) en UNA query de lote en vez
	// de una por simbolo -- ver el fallback de GetSnapshot.
	if len(needD1Fallback) > 0 {
		d1Series, err := s.repo.GetSeries(ctx, needD1Fallback, domain.D1, 1)
		if err == nil {
			for symbol, candles := range d1Series {
				if len(candles) == 0 {
					continue
				}
				snap := result[symbol]
				snap.PrevClose = candles[len(candles)-1].Close
				result[symbol] = snap
			}
		}
	}

	return result
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
