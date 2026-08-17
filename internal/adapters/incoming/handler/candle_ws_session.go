package handler

import (
	"context"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const initialHistoryBars = 500

type candleSubscribeRequest struct {
	Action    string `json:"action"`
	Symbol    string `json:"symbol"`
	Timeframe string `json:"timeframe"`
}

// wsSession es una conexion WS de /ws/candles -- multiplexa varias
// suscripciones symbol:timeframe sobre el mismo socket, igual que hace el
// cliente (ver candle-stream.service.ts). writeMu serializa las escrituras
// porque gorilla/websocket no admite dos goroutines escribiendo al mismo
// tiempo (aca compiten el loop de lectura y cada forwardLive en vivo).
type wsSession struct {
	conn        *websocket.Conn
	writeMu     sync.Mutex
	getCandles  in.GetCandlesService
	broadcaster *livecandles.Broadcaster

	mu   sync.Mutex
	subs map[string]func()
}

func newWSSession(conn *websocket.Conn, getCandles in.GetCandlesService, broadcaster *livecandles.Broadcaster) *wsSession {
	return &wsSession{conn: conn, getCandles: getCandles, broadcaster: broadcaster, subs: make(map[string]func())}
}

func (s *wsSession) run(ctx context.Context) {
	defer s.closeAll()
	for {
		var req candleSubscribeRequest
		if err := s.conn.ReadJSON(&req); err != nil {
			return
		}
		switch req.Action {
		case "subscribe":
			s.handleSubscribe(ctx, req.Symbol, req.Timeframe)
		case "unsubscribe":
			s.handleUnsubscribe(req.Symbol, req.Timeframe)
		}
	}
}

// handleSubscribe manda el historial de una sola vez (setData del lado del
// frontend) y se suscribe al Broadcaster del simbolo para reenviar en vivo
// la vela en formacion: M1 directa (el unico timeframe con feed propio) y
// los demas agregando las M1 del periodo actual -- todos los timeframes
// validos tienen vela en formacion en el grafico, no solo M1. Cualquier
// otro timeframe (los no soportados) responde error en vez de fallar en
// silencio, mismo criterio que /marketdata/timeframes.
func (s *wsSession) handleSubscribe(ctx context.Context, symbol, timeframe string) {
	key := symbol + ":" + timeframe
	s.mu.Lock()
	_, exists := s.subs[key]
	s.mu.Unlock()
	if exists {
		return
	}

	tf := domain.Timeframe(timeframe)
	if !tf.Valid() {
		s.sendJSON(dto.CandleControlMessage{Type: "error", Symbol: symbol, Timeframe: timeframe, Message: "timeframe no soportado todavia"})
		return
	}

	candles, err := s.getCandles.GetCandles(ctx, symbol, tf, initialHistoryBars, nil)
	if err != nil {
		s.sendJSON(dto.CandleControlMessage{Type: "error", Symbol: symbol, Timeframe: timeframe, Message: "no se pudo cargar el historial"})
		return
	}
	s.sendJSON(dto.CandleHistoryMessage{Type: "history", Symbol: symbol, Timeframe: timeframe, Bars: toBars(candles)})

	var lastTime int64
	if len(candles) > 0 {
		lastTime = candles[len(candles)-1].Timestamp.Unix()
	}

	// La vela en formacion arranca con las M1 del periodo actual ya
	// guardadas (las del broadcast solo llegan desde el momento de la
	// suscripcion) -- para M1 es nil: la vela del minuto en curso llega con
	// el primer tick.
	var seed *dto.CandleBar
	if tf != domain.M1 {
		seed = s.seedForming(ctx, symbol, tf)
	}

	ch, cancel := s.broadcaster.Subscribe(symbol)
	s.mu.Lock()
	s.subs[key] = cancel
	s.mu.Unlock()
	go s.forwardLive(ch, symbol, timeframe, lastTime, seed)
}

func (s *wsSession) handleUnsubscribe(symbol, timeframe string) {
	key := symbol + ":" + timeframe
	s.mu.Lock()
	cancel, exists := s.subs[key]
	delete(s.subs, key)
	s.mu.Unlock()
	if exists {
		cancel()
	}
}

// forwardLive reenvia las velas M1 del simbolo agregadas al periodo del
// timeframe suscrito: para M1 el periodo es el minuto de la vela misma
// (identidad), para H1/D1/etc. la vela en formacion se arma sumando las M1
// del periodo. Cada periodo nuevo emite la vela anterior como cerrada y
// arranca la siguiente en formacion -- el frontend hace update() por
// tiempo, asi que una vela cerrada repetida solo reemplaza su version.
// lastHistoryTime protege la serie: jamas se emite una vela anterior al
// ultimo bar del historial (lightweight-charts rompe con tiempos
// descendentes).
func (s *wsSession) forwardLive(ch <-chan domain.Candle, symbol, timeframe string, lastHistoryTime int64, seed *dto.CandleBar) {
	tf := domain.Timeframe(timeframe)
	agg := seed
	for c := range ch {
		if !c.IsComplete() {
			continue
		}
		period := formingPeriodStart(c.Timestamp, tf).Unix()
		if agg == nil || agg.Time != period {
			if agg != nil && agg.Time >= lastHistoryTime {
				s.sendBar(symbol, timeframe, *agg, true)
			}
			agg = &dto.CandleBar{Time: period, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Closed: false}
		} else {
			if c.High > agg.High {
				agg.High = c.High
			}
			if c.Low < agg.Low {
				agg.Low = c.Low
			}
			agg.Close = c.Close
			agg.Volume += c.Volume
		}
		if agg.Time >= lastHistoryTime {
			s.sendBar(symbol, timeframe, *agg, false)
		}
	}
}

// seedForming arma la vela en formacion del timeframe al momento de la
// suscripcion con las M1 del periodo actual ya guardadas en BD -- sin esto,
// la primera vela en formacion mostraria solo los ticks posteriores a la
// conexion (alta/cierre incompletos). GetCandles solo acota por "los N mas
// recientes" (before es un tope superior, no un minimo), asi que se toman
// las 2000 M1 mas nuevas y se filtran las del periodo; el periodo D1 actual
// nunca excede las ~1500 M1 de un dia completo.
func (s *wsSession) seedForming(ctx context.Context, symbol string, tf domain.Timeframe) *dto.CandleBar {
	start := formingPeriodStart(time.Now(), tf)
	candles, err := s.getCandles.GetCandles(ctx, symbol, domain.M1, 2000, nil)
	if err != nil || len(candles) == 0 {
		return nil
	}
	inPeriod := candles[:0]
	for _, c := range candles {
		if !c.Timestamp.Before(start) {
			inPeriod = append(inPeriod, c)
		}
	}
	if len(inPeriod) == 0 {
		return nil
	}
	first := inPeriod[0]
	bar := &dto.CandleBar{Time: start.Unix(), Open: first.Open, High: first.High, Low: first.Low, Close: first.Close, Volume: first.Volume, Closed: false}
	for _, c := range inPeriod[1:] {
		if c.High > bar.High {
			bar.High = c.High
		}
		if c.Low < bar.Low {
			bar.Low = c.Low
		}
		bar.Close = c.Close
		bar.Volume += c.Volume
	}
	return bar
}

// formingPeriodStart alinea el timestamp al inicio del periodo del timeframe
// con la misma convencion que las velas guardadas (dxFeed): intraday y
// diarios alineados a UTC, semana el lunes 00:00 UTC, mes el dia 1, anio el
// 1 de enero. La vela en formacion se estampa con este inicio para
// continuar la serie del historial.
func formingPeriodStart(t time.Time, tf domain.Timeframe) time.Time {
	u := t.UTC()
	switch tf {
	case domain.W1:
		daysSinceMonday := (int(u.Weekday()) + 6) % 7
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysSinceMonday)
	case domain.MO1, domain.MO3, domain.MO6:
		return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	case domain.Y1:
		return time.Date(u.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}
	d, err := tf.Duration()
	if err != nil || d <= 0 {
		return u
	}
	secs := int64(d.Seconds())
	return time.Unix(u.Unix()-u.Unix()%secs, 0).UTC()
}

func (s *wsSession) sendBar(symbol, timeframe string, bar dto.CandleBar, closed bool) {
	bar.Closed = closed
	s.sendJSON(dto.CandleBarMessage{Type: "bar", Symbol: symbol, Timeframe: timeframe, Bar: bar})
}

func (s *wsSession) closeAll() {
	s.mu.Lock()
	subs := s.subs
	s.subs = nil
	s.mu.Unlock()
	for _, cancel := range subs {
		cancel()
	}
	s.conn.Close()
}

func (s *wsSession) sendJSON(v any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.WriteJSON(v); err != nil {
		log.Error().Err(err).Msg("failed to write to candle ws client")
	}
}

func toBars(candles []domain.Candle) []dto.CandleBar {
	bars := make([]dto.CandleBar, len(candles))
	for i, c := range candles {
		bars[i] = toBar(c)
	}
	return bars
}

func toBar(c domain.Candle) dto.CandleBar {
	return dto.CandleBar{
		Time:   c.Timestamp.Unix(),
		Open:   c.Open,
		High:   c.High,
		Low:    c.Low,
		Close:  c.Close,
		Volume: c.Volume,
		Closed: true,
	}
}
