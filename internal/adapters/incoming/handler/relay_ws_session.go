package handler

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
)

type relaySubscribeRequest struct {
	Action string `json:"action"`
	Symbol string `json:"symbol"`
}

// relayWSSession es el WS minimo que comparten /ws/snapshot y
// /ws/fundamentals -- a diferencia de /ws/candles (wsSession en
// candle_ws_session.go), no arma historial ni agrega timeframes: solo
// reenvia tal cual cada Publish del Broadcaster[T] del simbolo suscripto.
// El keepalive/cierre (writeMu, ping, closeAll, sendJSON) vive en
// baseWSSession (mismo paquete), compartido con wsSession.
type relayWSSession[T any] struct {
	baseWSSession
	broadcaster *livecandles.Broadcaster[T]
	toMessage   func(symbol string, item T) any
}

func newRelayWSSession[T any](conn *websocket.Conn, broadcaster *livecandles.Broadcaster[T], toMessage func(string, T) any) *relayWSSession[T] {
	return &relayWSSession[T]{
		baseWSSession: newBaseWSSession(conn, "failed to write to relay ws client"),
		broadcaster:   broadcaster,
		toMessage:     toMessage,
	}
}

func (s *relayWSSession[T]) run(ctx context.Context) {
	defer s.closeAll()
	s.armKeepalive()
	go s.pingLoop()
	for {
		var req relaySubscribeRequest
		if err := s.conn.ReadJSON(&req); err != nil {
			return
		}
		switch req.Action {
		case "subscribe":
			s.handleSubscribe(req.Symbol)
		case "unsubscribe":
			s.handleUnsubscribe(req.Symbol)
		}
	}
}

func (s *relayWSSession[T]) handleSubscribe(symbol string) {
	s.mu.Lock()
	_, exists := s.subs[symbol]
	s.mu.Unlock()
	if exists {
		return
	}
	ch, cancel := s.broadcaster.Subscribe(symbol)
	s.mu.Lock()
	s.subs[symbol] = cancel
	s.mu.Unlock()
	go s.forward(ch, symbol)
}

func (s *relayWSSession[T]) handleUnsubscribe(symbol string) {
	s.mu.Lock()
	cancel, exists := s.subs[symbol]
	delete(s.subs, symbol)
	s.mu.Unlock()
	if exists {
		cancel()
	}
}

func (s *relayWSSession[T]) forward(ch <-chan T, symbol string) {
	for item := range ch {
		s.sendJSON(s.toMessage(symbol, item))
	}
}

