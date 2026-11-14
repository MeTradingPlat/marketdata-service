package handler

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/gorilla/websocket"
)

// pingInterval/pongWait: el dominio se expone via Cloudflare Tunnel (ver
// systemctl cloudflared.service en el VAIO) -- confirmado en vivo el
// 2026-08-24 que /ws/candles se cerraba solo cada ~125.6s de forma
// consistente pese a que ni el Gateway (esa ruta no tiene response-timeout
// propio) ni Echo/gorilla-websocket de este lado tienen ningun timeout
// configurado: Cloudflare corta una conexion que no ve trafico en unos
// ~100-125s. Un Ping cada 30s (bien por debajo del umbral observado)
// mantiene el tunel viendo actividad real. pongWait > pingInterval deja
// tolerancia a 1-2 pings perdidos antes de dar la conexion por muerta.
const (
	pingInterval = 30 * time.Second
	pongWait     = 90 * time.Second
)

type candleSubscribeRequest struct {
	Action     string   `json:"action"`
	Symbol     string   `json:"symbol"`
	Symbols    []string `json:"symbols"`
	Timeframe  string   `json:"timeframe"`
	Bars       int      `json:"bars"`
	ClosedOnly bool     `json:"closedOnly"`
}

// wsSession es una conexion WS de /ws/candles -- multiplexa varias
// suscripciones symbol:timeframe sobre el mismo socket, igual que hace el
// cliente (ver candle-stream.service.ts). El resto del keepalive/cierre
// (writeMu, ping, closeAll, sendJSON) vive en baseWSSession (mismo paquete),
// compartido con relayWSSession[T].
type wsSession struct {
	baseWSSession
	getCandles in.GetCandlesService
	current    in.GetCurrentCandleService
	hub        *candleAggregateHub
}

func newWSSession(conn *websocket.Conn, getCandles in.GetCandlesService, current in.GetCurrentCandleService, hub *candleAggregateHub) *wsSession {
	return &wsSession{
		baseWSSession: newBaseWSSession(conn, "failed to write to candle ws client"),
		getCandles:    getCandles,
		current:       current,
		hub:           hub,
	}
}

func (s *wsSession) run(ctx context.Context) {
	defer s.closeAll()
	s.armKeepalive()
	go s.pingLoop()
	go s.dispatchLoop()
	for {
		var req candleSubscribeRequest
		if err := s.conn.ReadJSON(&req); err != nil {
			return
		}
		switch req.Action {
		case "subscribe":
			// Corre en su propia goroutine -- handleSubscribe/handleSubscribeBatch
			// hacen I/O sincrono (GetCandles/GetCandlesBatch) que puede tardar,
			// y esta goroutine (run) es la UNICA que lee del socket: si el
			// handler corriera aca mismo, un lote grande (ej. todo el universo
			// de un escaner sin pre-filtros, miles de simbolos de golpe)
			// bloqueaba ReadJSON el tiempo entero que tarda ese fetch, sin
			// volver a leer nada del cliente -- incluidos los PING de
			// keepalive, que terminaban venciendo su propio timeout y
			// tumbando la conexion antes de que la suscripcion llegara a
			// completarse. Confirmado en vivo 2026-09-16.
			symbols, timeframe, bars, closedOnly := req.Symbols, req.Timeframe, historyBars(req.Bars), req.ClosedOnly
			if len(symbols) > 0 {
				go s.handleSubscribeBatch(ctx, symbols, timeframe, bars, closedOnly)
			} else {
				symbol := req.Symbol
				go s.handleSubscribe(ctx, symbol, timeframe, bars, closedOnly)
			}
		case "unsubscribe":
			if len(req.Symbols) > 0 {
				for _, symbol := range req.Symbols {
					s.handleUnsubscribe(symbol, req.Timeframe)
				}
			} else {
				s.handleUnsubscribe(req.Symbol, req.Timeframe)
			}
		}
	}
}
