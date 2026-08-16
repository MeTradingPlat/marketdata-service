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
// frontend) y, solo para M1 -- el unico timeframe con feed en vivo real --
// se suscribe al Broadcaster para reenviar cada vela nueva como 'bar'.
// H1/D1 sirven historial igual (ya tenemos esos datos) pero sin push en
// vivo; cualquier otro timeframe (los 18 sin agregar todavia) responde
// error en vez de fallar en silencio, mismo criterio que /marketdata/timeframes.
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

	candles, err := s.getCandles.GetCandles(ctx, symbol, tf, initialHistoryBars)
	if err != nil {
		s.sendJSON(dto.CandleControlMessage{Type: "error", Symbol: symbol, Timeframe: timeframe, Message: "no se pudo cargar el historial"})
		return
	}
	s.sendJSON(dto.CandleHistoryMessage{Type: "history", Symbol: symbol, Timeframe: timeframe, Bars: toBars(candles)})

	if tf != domain.M1 {
		return
	}

	ch, cancel := s.broadcaster.Subscribe(symbol)
	s.mu.Lock()
	s.subs[key] = cancel
	s.mu.Unlock()
	go s.forwardLive(ch, symbol, timeframe)
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

func (s *wsSession) forwardLive(ch <-chan domain.Candle, symbol, timeframe string) {
	for c := range ch {
		s.sendJSON(dto.CandleBarMessage{Type: "bar", Symbol: symbol, Timeframe: timeframe, Bar: toBar(c)})
	}
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
