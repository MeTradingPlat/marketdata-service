package handler

import (
	"context"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type relaySubscribeRequest struct {
	Action string `json:"action"`
	Symbol string `json:"symbol"`
}

// relayWSSession es el WS minimo que comparten /ws/snapshot y
// /ws/fundamentals -- a diferencia de /ws/candles (wsSession en
// candle_ws_session.go), no arma historial ni agrega timeframes: solo
// reenvia tal cual cada Publish del Broadcaster[T] del simbolo suscripto.
// pingInterval/pongWait son las constantes de candle_ws_session.go (mismo
// paquete, mismo motivo: mantener vivo el tunel de Cloudflare).
type relayWSSession[T any] struct {
	conn        *websocket.Conn
	writeMu     sync.Mutex
	broadcaster *livecandles.Broadcaster[T]
	toMessage   func(symbol string, item T) any

	mu   sync.Mutex
	subs map[string]func()
}

func newRelayWSSession[T any](conn *websocket.Conn, broadcaster *livecandles.Broadcaster[T], toMessage func(string, T) any) *relayWSSession[T] {
	return &relayWSSession[T]{conn: conn, broadcaster: broadcaster, toMessage: toMessage, subs: make(map[string]func())}
}

func (s *relayWSSession[T]) run(ctx context.Context) {
	defer s.closeAll()
	_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
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

func (s *relayWSSession[T]) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
			return
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

func (s *relayWSSession[T]) closeAll() {
	s.mu.Lock()
	subs := s.subs
	s.subs = nil
	s.mu.Unlock()
	for _, cancel := range subs {
		cancel()
	}
	s.conn.Close()
}

func (s *relayWSSession[T]) sendJSON(v any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.WriteJSON(v); err != nil {
		log.Error().Err(err).Msg("failed to write to relay ws client")
	}
}
