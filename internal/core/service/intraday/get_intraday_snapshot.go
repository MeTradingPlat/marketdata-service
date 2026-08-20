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

// GetSnapshotsBatch es GetSnapshot para un lote de simbolos leyendo las
// sesiones del dia desde el SnapshotTracker en memoria (actualizado vela a
// vela por el streaming en vivo, ver RecordClosedCandle) en vez de
// recalcularlas desde disco en cada request -- confirmado en vivo el
// 2026-08-20: incluso ya en un solo query de lote (ver el batch de
// GetIntradaySessionsBatch), agregar el chunk M1 de hoy (8.8M+ filas, todo
// el universo) para 8861 simbolos a la vez seguia tardando 90s+ por como el
// planner recorre un chunk ordenado por TIEMPO cuando se filtra por
// SIMBOLO. Solo cae a BD (GetIntradaySessionsBatch, ver el fallback abajo)
// para simbolos sin ninguna vela registrada todavia hoy en el tracker --
// tipico justo tras un reinicio, antes de que el seed y el streaming en
// vivo los cubran.
func (s *getIntradaySnapshotService) GetSnapshotsBatch(ctx context.Context, symbols []string, knownPrevCloses map[string]float64) map[string]domain.IntradaySnapshot {
	result := make(map[string]domain.IntradaySnapshot, len(symbols))
	if len(symbols) == 0 {
		return result
	}

	sessions := s.tracker.SnapshotBatch(symbols)
	missingSessions := make([]string, 0)
	for _, sym := range symbols {
		if _, ok := sessions[sym]; !ok {
			missingSessions = append(missingSessions, sym)
		}
	}
	if len(missingSessions) > 0 {
		if fallback, err := s.repo.GetIntradaySessionsBatch(ctx, missingSessions); err == nil {
			for sym, snap := range fallback {
				sessions[sym] = snap
			}
		}
	}

	now := time.Now()
	needD1Fallback := make([]string, 0)

	// prevCloses arranca con lo que el caller ya sabe (RefreshPrevClose lo
	// calcula una vez por ventana de mantenimiento para el universo entero,
	// ver domain.Fundamentals.PrevClose) -- solo se consulta la subasta/D1
	// para los simbolos SIN ese dato: confirmado en vivo el 2026-08-20, con
	// el universo entero recien desplegado (nada en knownPrevCloses todavia)
	// GetPreviousSessionCloseBatch + su fallback D1 seguian tardando 70s+
	// pese a que RefreshPrevClose ya habia calculado exactamente lo mismo
	// minutos antes en la ventana de mantenimiento.
	prevCloses := make(map[string]float64, len(symbols))
	needPrevClose := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		if pc, ok := knownPrevCloses[sym]; ok {
			prevCloses[sym] = pc
		} else {
			needPrevClose = append(needPrevClose, sym)
		}
	}
	var err error
	if len(needPrevClose) > 0 {
		loc, locErr := time.LoadLocation("America/New_York")
		if locErr == nil {
			nowET := now.In(loc)
			openET := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
			fromDB, dbErr := s.repo.GetPreviousSessionCloseBatch(ctx, needPrevClose, openET)
			if dbErr == nil {
				for sym, pc := range fromDB {
					prevCloses[sym] = pc
				}
			}
		}
	}

	// needM1Fallback: simbolos sin vela en formacion en el gateway NI ultima
	// M1 cerrada en el tracker (en vivo todavia no suscrito Y sin ningun
	// tick registrado hoy -- tipico justo tras un deploy/rollout, antes de
	// que el primer tick de cada simbolo llegue). Confirmado en vivo el
	// 2026-08-19/20: tanto llamar GetCandles(M1) simbolo por simbolo como
	// GetSeriesPriority en lote para el universo ENTERO recien desplegado
	// tardaban 80-90s+ -- el tracker.LastClose cubre el caso comun (el
	// simbolo ya cerro al menos una M1 hoy) en memoria; solo lo que ni
	// siquiera eso tiene cae a BD.
	needM1Fallback := make([]string, 0)
	live := make(map[string]domain.Candle, len(symbols))
	for _, symbol := range symbols {
		if current, ok := s.gateway.CurrentCandle(symbol); ok {
			live[symbol] = current
		} else if _, _, ok := s.tracker.LastClose(symbol); !ok {
			needM1Fallback = append(needM1Fallback, symbol)
		}
	}
	var m1Fallback map[string][]domain.Candle
	if len(needM1Fallback) > 0 {
		m1Fallback, err = s.repo.GetSeriesPriority(ctx, needM1Fallback, domain.M1, 1)
		if err != nil {
			m1Fallback = map[string][]domain.Candle{}
		}
	}

	for _, symbol := range symbols {
		snap, ok := sessions[symbol]
		if !ok {
			snap = domain.IntradaySnapshot{Symbol: symbol}
		}
		snap.Symbol = symbol
		snap.AsOf = now

		if current, ok := live[symbol]; ok {
			snap.CurrentPrice = current.Close
			snap.CurrentVolume = current.Volume
			mergeFormingCandle(&snap, current)
		} else if price, volume, ok := s.tracker.LastClose(symbol); ok {
			snap.CurrentPrice = price
			snap.CurrentVolume = volume
		} else if lastM1 := m1Fallback[symbol]; len(lastM1) > 0 {
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
		d1Series, err := s.repo.GetSeriesPriority(ctx, needD1Fallback, domain.D1, 1)
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
