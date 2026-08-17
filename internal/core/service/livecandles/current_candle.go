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
// ticks sin esperar el guardado), o una plana al ultimo cierre si el minuto
// no tuvo trades. Para los timeframes derivados agrega las M1 del periodo
// desde la BD mas la M1 en curso del pool, y plana al ultimo cierre si el
// periodo no tiene datos todavia (ej. D1 antes de la apertura).
type CurrentCandleService struct {
	candles in.GetCandlesService
	gateway out.MarketDataGateway
}

// El provider devuelve la INTERFAZ (patron dig del proyecto, igual que
// NewGetCandlesService): dig no resuelve una interfaz desde un provider de
// tipo concreto -- con el return concreto el arranque paniqueaba con
// "missing type: in.GetCurrentCandleService (did you mean
// *livecandles.CurrentCandleService?)".
func NewCurrentCandleService(candles in.GetCandlesService, gateway out.MarketDataGateway) in.GetCurrentCandleService {
	return &CurrentCandleService{candles: candles, gateway: gateway}
}

// var _ pin en tiempo de compilacion: dig falla en RUNTIME si el metodo no
// matchea la interfaz (panic "missing dependencies" en el arranque -- paso
// en el deploy a6b8789), esto lo atrapa en build.
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
		if liveBar != nil {
			return liveBar, nil
		}
		return s.flatAtLastClose(ctx, symbol, period), nil
	}

	// Timeframes derivados: agregar las M1 del periodo guardadas en BD y la
	// M1 en curso del pool (que aun no se guardo, refresca el cierre/mechas).
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
	if bar == nil {
		bar = s.flatAtLastClose(ctx, symbol, period)
	}
	return bar, nil
}

// flatAtLastClose arma la vela en formacion "plana" al ultimo cierre
// conocido (open=high=low=close) para el periodo indicado -- el grafico
// siempre tiene la vela en formacion presente aunque el periodo no tenga
// trades todavia; el primer tick real la corrige (mismo patron que
// TradingView para periodos muertos). nil si no hay ningun cierre previo.
func (s *CurrentCandleService) flatAtLastClose(ctx context.Context, symbol string, period time.Time) *dto.CandleBar {
	m1s, err := s.candles.GetCandles(ctx, symbol, domain.M1, 1, nil)
	if err != nil || len(m1s) == 0 {
		return nil
	}
	last := m1s[len(m1s)-1]
	if last.Close == 0 {
		return nil
	}
	return &dto.CandleBar{Time: period.Unix(), Open: last.Close, High: last.Close, Low: last.Close, Close: last.Close, Closed: false}
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
// 1 de enero. La vela en formacion se estampa con este inicio para
// continuar la serie del historial.
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
	}
	// Base y derivados intraday/diarios: alinear por duracion exacta en
	// segundos (M5 -> 5 min, H4 -> 4 h, D2 -> 2 dias...). La duracion de los
	// derivados sale de Aggregation().approxPeriod, que es exacta para
	// todos estos y reproduce el time_bucket de la BD (misma convencion
	// UTC confirmada contra dxFeed).
	d, err := tf.Duration()
	if err != nil {
		_, _, approx, ok := tf.Aggregation()
		if !ok || approx <= 0 {
			return u
		}
		d = approx
	}
	secs := int64(d.Seconds())
	return time.Unix(u.Unix()-u.Unix()%secs, 0).UTC()
}

// FormingPeriodEnd es el limite exclusivo del periodo que arranca en start
// (calendario para semana/mes/anio, duracion exacta para el resto).
func FormingPeriodEnd(start time.Time, tf domain.Timeframe) time.Time {
	switch tf {
	case domain.W1:
		return start.AddDate(0, 0, 7)
	case domain.MO1:
		return start.AddDate(0, 1, 0)
	case domain.MO3:
		return start.AddDate(0, 3, 0)
	case domain.MO6:
		return start.AddDate(0, 6, 0)
	case domain.Y1:
		return start.AddDate(1, 0, 0)
	}
	d, err := tf.Duration()
	if err != nil {
		_, _, approx, ok := tf.Aggregation()
		if !ok || approx <= 0 {
			return start
		}
		d = approx
	}
	return start.Add(d)
}
