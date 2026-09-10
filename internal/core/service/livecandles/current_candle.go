package livecandles

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

// CurrentCandleService arma la vela EN FORMACION del simbolo+timeframe --
// exclusivamente para el grafico en vivo: signal-processing consume velas
// cerradas de la BD y jamás pasa por acá (la vela en formacion no se
// guarda; el guardado solo ocurre al cerrar el periodo). Para M1 devuelve
// la vela del minuto en curso tal cual la fusiona el pool (la mas fiel:
// ticks sin esperar el guardado), o nil si el minuto no tuvo ningun trade
// todavia. Para los timeframes derivados agrega las M1 reales del periodo
// desde la BD mas la M1 en curso del pool, y nil si el periodo no tiene
// ningun dato real todavia (ej. D1 antes de la apertura).
type CurrentCandleService struct {
	candles in.GetCandlesService
	gateway out.MarketDataGateway
}

// El provider devuelve la INTERFAZ (patron dig del proyecto, igual que
// NewGetCandlesService).
func NewCurrentCandleService(candles in.GetCandlesService, gateway out.MarketDataGateway) in.GetCurrentCandleService {
	return &CurrentCandleService{candles: candles, gateway: gateway}
}

var _ in.GetCurrentCandleService = (*CurrentCandleService)(nil)

func (s *CurrentCandleService) GetCurrentCandle(ctx context.Context, symbol string, tf domain.Timeframe) (*dto.CandleBar, error) {
	period := FormingPeriodStart(time.Now(), tf)
	end := FormingPeriodEnd(period, tf)

	live, ok := s.gateway.CurrentCandle(symbol)
	var liveBar *dto.CandleBar
	if ok && live.IsComplete() && !live.Timestamp.Before(period) && live.Timestamp.Before(end) {
		b := toFormingBar(live)
		b.Time = period.Unix()
		liveBar = b
	}

	if tf == domain.M1 {
		return liveBar, nil
	}

	// Timeframes derivados: agregar las M1 REALES del periodo guardadas en
	// BD mas la M1 en curso del pool (que aun no se guardo, refresca el
	// cierre/mechas). Si ninguna de las dos existe todavia, bar se queda
	// nil -- sin inventar una plana, el periodo simplemente no tiene vela
	// en formacion hasta que llegue el primer dato real.
	m1s, err := s.candles.GetCandles(ctx, symbol, domain.M1, 2000, nil)
	if err != nil {
		return nil, fmt.Errorf("loading M1 bars for forming %s %s: %w", symbol, tf, err)
	}
	var bar *dto.CandleBar
	for _, c := range m1s {
		if !c.Timestamp.Before(period) && c.Timestamp.Before(end) {
			bar = foldM1Bar(bar, c, period)
		}
	}
	if liveBar != nil {
		bar = foldBar(bar, liveBar, period)
	}
	return bar, nil
}

func toFormingBar(c domain.Candle) *dto.CandleBar {
	return &dto.CandleBar{Time: c.Timestamp.Unix(), Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Closed: false}
}

func foldM1Bar(bar *dto.CandleBar, c domain.Candle, period time.Time) *dto.CandleBar {
	b := bar
	if b == nil {
		b = &dto.CandleBar{Time: period.Unix(), Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Closed: false}
		return b
	}
	return foldBar(b, toFormingBar(c), period)
}

func foldBar(bar *dto.CandleBar, next *dto.CandleBar, period time.Time) *dto.CandleBar {
	if bar == nil {
		b := *next
		b.Time = period.Unix()
		b.Closed = false
		return &b
	}
	if next.High > bar.High {
		bar.High = next.High
	}
	if next.Low < bar.Low {
		bar.Low = next.Low
	}
	bar.Close = next.Close
	bar.Volume += next.Volume
	return bar
}

// FormingPeriodStart alinea el timestamp al inicio del periodo del timeframe
// con la misma convencion que las velas guardadas (dxFeed): intraday y
// diarios alineados a UTC, semana el lunes 00:00 UTC, mes el dia 1, anio el
// 1 de enero.
func FormingPeriodStart(t time.Time, tf domain.Timeframe) time.Time {
	u := t.UTC()
	switch tf {
	case domain.W1:
		daysSinceMonday := (int(u.Weekday()) + 6) % 7
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysSinceMonday)
	case domain.MO1:
		return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	case domain.MO3:
		month := (int(u.Month())-1)/3*3 + 1
		return time.Date(u.Year(), time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	case domain.MO6:
		month := 1
		if u.Month() >= time.July {
			month = 7
		}
		return time.Date(u.Year(), time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	case domain.Y1:
		return time.Date(u.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default:
		d, err := tf.Duration()
		if err == nil {
			return u.Truncate(d)
		}
		if _, _, approx, ok := tf.Aggregation(); ok && approx > 0 {
			return u.Truncate(approx)
		}
		return u.Truncate(time.Minute)
	}
}

// FormingPeriodEnd devuelve el fin del periodo que arranco en periodStart.
func FormingPeriodEnd(periodStart time.Time, tf domain.Timeframe) time.Time {
	switch tf {
	case domain.W1:
		return periodStart.AddDate(0, 0, 7)
	case domain.MO1:
		return periodStart.AddDate(0, 1, 0)
	case domain.MO3:
		return periodStart.AddDate(0, 3, 0)
	case domain.MO6:
		return periodStart.AddDate(0, 6, 0)
	case domain.Y1:
		return periodStart.AddDate(1, 0, 0)
	default:
		d, err := tf.Duration()
		if err == nil {
			return periodStart.Add(d)
		}
		if _, _, approx, ok := tf.Aggregation(); ok && approx > 0 {
			return periodStart.Add(approx)
		}
		return periodStart.Add(time.Minute)
	}
}
