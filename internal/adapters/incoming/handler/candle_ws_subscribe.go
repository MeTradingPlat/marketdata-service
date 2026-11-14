package handler

import (
	"context"
	"sync/atomic"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

// handleSubscribe manda el historial de una sola vez (setData del lado del
// frontend) y se suscribe al Broadcaster del simbolo para reenviar en vivo
// la vela en formacion: M1 directa (el unico timeframe con feed propio) y
// los demas agregando las M1 del periodo actual -- todos los timeframes
// validos tienen vela en formacion en el grafico, no solo M1. Cualquier
// otro timeframe (los no soportados) responde error en vez de fallar en
// silencio, mismo criterio que /marketdata/timeframes.
func (s *wsSession) handleSubscribe(ctx context.Context, symbol, timeframe string, bars int, closedOnly bool) {
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

	candles, err := s.getCandles.GetCandles(ctx, symbol, tf, bars, nil)
	if err != nil {
		s.sendJSON(dto.CandleControlMessage{Type: "error", Symbol: symbol, Timeframe: timeframe, Message: "no se pudo cargar el historial"})
		return
	}
	s.seedAndSubscribe(ctx, symbol, timeframe, tf, candles, closedOnly)
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
func (s *wsSession) handleSubscribeBatch(ctx context.Context, symbols []string, timeframe string, bars int, closedOnly bool) {
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

	candlesBatch := s.getCandles.GetCandlesBatch(ctx, pending, tf, bars)
	for _, symbol := range pending {
		s.seedAndSubscribe(ctx, symbol, timeframe, tf, candlesBatch[symbol], closedOnly)
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
func (s *wsSession) seedAndSubscribe(ctx context.Context, symbol, timeframe string, tf domain.Timeframe, candles []domain.Candle, closedOnly bool) {
	var lastTime int64
	if len(candles) > 0 {
		lastTime = candles[len(candles)-1].Timestamp.Unix()
	}

	bars := toBars(candles)
	var seed *dto.CandleBar
	if s.current != nil && !closedOnly {
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

	// El chequeo contra lastTime protege la serie DE ESTA sesion: jamas
	// reenvia una vela anterior al ultimo bar del historial que ya mando en
	// el mensaje "history" (otra sesion pudo pedir su propio historial en un
	// instante levemente distinto). Corre en la goroutine que publica el
	// tick, no en la de esta sesion -- debe ser rapido y no bloqueante.
	var closedSeq atomic.Int64
	cancel := s.hub.Subscribe(ctx, symbol, timeframe, tf, func(bar dto.CandleBar) {
		if bar.Time < lastTime || (closedOnly && !bar.Closed) {
			return
		}
		if bar.Closed && !bar.Corrected {
			bar.Seq = closedSeq.Add(1)
		}
		message := dto.CandleBarMessage{Type: "bar", Symbol: symbol, Timeframe: timeframe, Bar: bar}
		if bar.Closed {
			s.publishReliable(message)
			return
		}
		s.publish(message)
	})
	s.mu.Lock()
	if s.subs == nil {
		// La sesion ya se esta cerrando -- closeAll() nilea s.subs bajo el
		// mismo mutex para señalar justo esto. Como el handler ahora corre
		// en su propia goroutine (ver run()), puede seguir en vuelo cuando
		// run() ya salio del loop de lectura y disparo closeAll() por su
		// cuenta. Sin este chequeo, la asignacion de mas abajo entraba en
		// un mapa nil y tumbaba el proceso entero con un panic (confirmado
		// en CI 2026-09-16). Nadie va a llamar a este cancel() -- se hace
		// aca para no dejar el worker compartido del hub huerfano.
		s.mu.Unlock()
		cancel()
		return
	}
	s.subs[symbol+":"+timeframe] = cancel
	s.mu.Unlock()
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
