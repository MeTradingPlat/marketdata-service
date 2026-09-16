package handler

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
)

const initialHistoryBars = 500

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
	Action    string   `json:"action"`
	Symbol    string   `json:"symbol"`
	Symbols   []string `json:"symbols"`
	Timeframe string   `json:"timeframe"`
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
			symbols, timeframe := req.Symbols, req.Timeframe
			if len(symbols) > 0 {
				go s.handleSubscribeBatch(ctx, symbols, timeframe)
			} else {
				symbol := req.Symbol
				go s.handleSubscribe(ctx, symbol, timeframe)
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

// handleSubscribe manda el historial de una sola vez (setData del lado del
// frontend) y se suscribe al Broadcaster del simbolo para reenviar en vivo
// la vela en formacion: M1 directa (el unico timeframe con feed propio) y
// los demas agregando las M1 del periodo actual -- todos los timeframes
// validos tienen vela en formacion en el grafico, no solo M1. Cualquier
// otro timeframe (los no soportados) responde error en vez de fallar en
// silencio, mismo criterio que /marketdata/timeframes.
func (s *wsSession) handleSubscribe(ctx context.Context, symbol, timeframe string) {
	if !domain.ValidSymbolFormat(symbol) {
		s.sendJSON(dto.CandleControlMessage{Type: "error", Symbol: symbol, Timeframe: timeframe, Message: "simbolo invalido"})
		return
	}
	if s.alreadySubscribed(symbol, timeframe) {
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
	s.seedAndSubscribe(ctx, symbol, timeframe, tf, candles)
}

// handleSubscribeBatch es handleSubscribe para varios simbolos del MISMO
// timeframe de una sola vez -- pensado para un cliente que necesita
// suscribir de golpe cientos o miles de simbolos (ej.
// signal-processing-service al arrancar el dia, ver RealtimeCandleClient),
// en vez de un mensaje por simbolo: antes de esto cada suscripcion
// disparaba su propio GetCandles individual contra el mismo socket, en
// fila -- con miles de simbolos de golpe eso tardaba un buen rato en
// ponerse al dia. GetCandlesBatch trae el historial de todos en una sola
// consulta, igual que ya hace /marketdata/historical/batch por REST.
func (s *wsSession) handleSubscribeBatch(ctx context.Context, symbols []string, timeframe string) {
	tf := domain.Timeframe(timeframe)
	if !tf.Valid() {
		s.sendJSON(dto.CandleControlMessage{Type: "error", Timeframe: timeframe, Message: "timeframe no soportado todavia"})
		return
	}

	pending := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		if !domain.ValidSymbolFormat(symbol) {
			s.sendJSON(dto.CandleControlMessage{Type: "error", Symbol: symbol, Timeframe: timeframe, Message: "simbolo invalido"})
			continue
		}
		if !s.alreadySubscribed(symbol, timeframe) {
			pending = append(pending, symbol)
		}
	}
	if len(pending) == 0 {
		return
	}

	candlesBatch := s.getCandles.GetCandlesBatch(ctx, pending, tf, initialHistoryBars)
	for _, symbol := range pending {
		s.seedAndSubscribe(ctx, symbol, timeframe, tf, candlesBatch[symbol])
	}
}

func (s *wsSession) alreadySubscribed(symbol, timeframe string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.subs[symbol+":"+timeframe]
	return exists
}

// seedAndSubscribe manda el historial (con la vela en formacion sembrada
// desde GetCurrentCandle DENTRO del mismo mensaje -- el grafico/consumidor
// no espera un mensaje aparte para tenerla) y engancha la sesion al hub
// compartido -- ultimo paso comun entre una suscripcion individual y una
// en lote.
func (s *wsSession) seedAndSubscribe(ctx context.Context, symbol, timeframe string, tf domain.Timeframe, candles []domain.Candle) {
	var lastTime int64
	if len(candles) > 0 {
		lastTime = candles[len(candles)-1].Timestamp.Unix()
	}

	bars := toBars(candles)
	var seed *dto.CandleBar
	if s.current != nil {
		var err error
		seed, err = s.current.GetCurrentCandle(ctx, symbol, tf)
		if err != nil {
			seed = nil
		}
	}
	if seed != nil && seed.Time >= lastTime {
		bars = append(bars, *seed)
	}
	s.sendJSON(dto.CandleHistoryMessage{Type: "history", Symbol: symbol, Timeframe: timeframe, Bars: bars})

	ch, cancel := s.hub.Subscribe(ctx, symbol, timeframe, tf)
	s.mu.Lock()
	s.subs[symbol+":"+timeframe] = cancel
	s.mu.Unlock()
	go s.forwardLive(ch, symbol, timeframe, lastTime)
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

// seedAggregate arranca el agregado compartido (candleAggregateHub) del
// primer suscriptor de un (symbol, timeframe) con la vela en formacion que
// ya existia -- sin esto, el hub creado recien perderia el progreso que
// GetCurrentCandle ya tenia hasta que llegue el proximo tick real.
func seedAggregate(seed *dto.CandleBar, tf domain.Timeframe, now time.Time) *dto.CandleBar {
	if seed == nil {
		return nil
	}
	start := *seed
	start.Time = livecandles.FormingPeriodStart(now, tf).Unix()
	return &start
}

// forwardLive reenvia lo que ya viene agregado del hub compartido (ver
// candle_aggregate_hub.go) -- esta sesion ya no agrega nada por su cuenta.
// lastHistoryTime protege la serie DE ESTA sesion en particular: jamas
// reenvia una vela anterior al ultimo bar del historial que ya le mando en
// el mensaje "history" (otra sesion pudo pedir su propio historial en un
// instante levemente distinto).
func (s *wsSession) forwardLive(ch <-chan dto.CandleBar, symbol, timeframe string, lastHistoryTime int64) {
	for bar := range ch {
		if bar.Time < lastHistoryTime {
			continue
		}
		s.sendJSON(dto.CandleBarMessage{Type: "bar", Symbol: symbol, Timeframe: timeframe, Bar: bar})
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
