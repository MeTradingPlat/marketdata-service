package handler

import (
	"context"
	"sync"

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
	current     in.GetCurrentCandleService
	broadcaster *livecandles.Broadcaster

	mu   sync.Mutex
	subs map[string]func()
}

func newWSSession(conn *websocket.Conn, getCandles in.GetCandlesService, current in.GetCurrentCandleService, broadcaster *livecandles.Broadcaster) *wsSession {
	return &wsSession{conn: conn, getCandles: getCandles, current: current, broadcaster: broadcaster, subs: make(map[string]func())}
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

	// La vela en formacion se siembra al instante desde el servicio (M1 del
	// pool, derivados agregando las M1 del periodo, plana al ultimo cierre
	// si el periodo no tiene datos) -- el grafico NO espera al primer tick
	// para dibujarla. La agregacion en vivo sigue desde ahi (forwardLive).
	var seed *dto.CandleBar
	if s.current != nil {
		seed, err = s.current.GetCurrentCandle(ctx, symbol, tf)
		if err != nil {
			seed = nil
		}
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
		// Un tick puede traer OHLC parcial (minuto sin trades, primer evento
		// de un periodo) -- se dibuja igual (plano) y el siguiente tick lo
		// completa; descartar por OHLC incompleto dejaba huecos en la serie.
		if c.Close == 0 {
			continue
		}
		period := livecandles.FormingPeriodStart(c.Timestamp, tf).Unix()
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
