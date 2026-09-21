package handler

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// baseWSSession factoriza lo que comparten TODAS las sesiones WS del
// servicio (wsSession de /ws/candles, relayWSSession[T] de /ws/snapshot y
// /ws/fundamentals): la conexion, el mutex de escritura (gorilla/websocket
// no admite dos goroutines escribiendo al mismo tiempo), el registro de
// suscripciones activas y el keepalive contra el Cloudflare Tunnel (ver
// pingInterval/pongWait en candle_ws_session.go).
// outboundBuffer: margen para que una sesion con miles de suscripciones a
// la vez (ej. un escaner sin pre-filtros en tiempo real sobre el universo
// completo) no descarte barras de golpe apenas arranca -- un publicador
// nunca debe bloquearse por esto (ver publish), asi que un valor generoso
// aca solo cuesta memoria de un unico canal por sesion, no una goroutina por
// suscripcion como antes.
const outboundBuffer = 2048

const reliablePublishTimeout = 10 * time.Second

type baseWSSession struct {
	conn    *websocket.Conn
	writeMu sync.Mutex

	mu   sync.Mutex
	subs map[string]func()

	// out/done sostienen el UNICO despachador de esta sesion (ver
	// dispatchLoop) -- reemplaza una goroutine dedicada por cada
	// suscripcion symbol:timeframe por una sola por conexion, sin importar
	// cuantos simbolos suscriba esa sesion. Confirmado en vivo el
	// 2026-09-17: esa multiplicacion (no el volumen de datos) era la causa
	// del OOM de marketdata-service.
	out  chan any
	done chan struct{}

	// errContext identifica la sesion en el log de un sendJSON fallido --
	// unico dato que de verdad difiere entre wsSession y relayWSSession[T]
	// en esta parte compartida.
	errContext string
}

func newBaseWSSession(conn *websocket.Conn, errContext string) baseWSSession {
	return baseWSSession{
		conn:       conn,
		subs:       make(map[string]func()),
		out:        make(chan any, outboundBuffer),
		done:       make(chan struct{}),
		errContext: errContext,
	}
}

// dispatchLoop es el UNICO lector de `out` de esta sesion -- debe arrancarse
// una vez desde run() antes de aceptar suscripciones. Termina cuando
// closeAll() cierra `done`.
func (s *baseWSSession) dispatchLoop() {
	for {
		select {
		case v := <-s.out:
			s.sendJSON(v)
		case <-s.done:
			return
		}
	}
}

// publish encola v para el dispatchLoop de esta sesion sin bloquear al
// publicador (un socket lento nunca debe frenar al resto, mismo criterio
// que ya tenia el Broadcaster por canal) -- lo llaman los callbacks
// registrados via Broadcaster.Subscribe/candleAggregateHub.Subscribe, que
// corren en la goroutine de quien publica el dato, no en la de esta sesion.
func (s *baseWSSession) publish(v any) {
	select {
	case s.out <- v:
	default:
	}
}

// publishReliable es publish para lo que NO puede perderse (una vela cerrada):
// si la cola de la sesion esta llena, no descarta en silencio -- reintenta en
// una goroutine acotada por reliablePublishTimeout y deja un aviso en el log.
func (s *baseWSSession) publishReliable(v any) {
	select {
	case s.out <- v:
		return
	default:
	}
	log.Warn().Msg("ws session outbound queue full, delaying a closed candle instead of dropping it")
	go func() {
		select {
		case s.out <- v:
		case <-s.done:
		case <-time.After(reliablePublishTimeout):
			log.Error().Msg("ws session outbound queue stayed full, closed candle dropped")
		}
	}()
}

// armKeepalive arma el deadline de lectura y el pong handler que lo
// renueva -- debe llamarse antes de pingLoop al arrancar run().
func (s *baseWSSession) armKeepalive() {
	_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
}

// pingLoop mantiene el tunel de Cloudflare viendo trafico real (ver
// pingInterval) -- WriteControl es seguro de llamar en paralelo con
// WriteJSON/WriteMessage (godoc de gorilla/websocket: "Close and
// WriteControl methods can be called concurrently with all other
// methods"), no necesita competir por writeMu. Un error de escritura (el
// socket ya se cerro) simplemente termina el loop -- closeAll() ya se
// encarga de liberar todo lo demas cuando run() retorna.
func (s *baseWSSession) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
			return
		}
	}
}

func (s *baseWSSession) closeAll() {
	s.mu.Lock()
	subs := s.subs
	s.subs = nil
	s.mu.Unlock()
	for _, cancel := range subs {
		cancel()
	}
	close(s.done)
	s.conn.Close()
}

func (s *baseWSSession) sendJSON(v any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.WriteJSON(v); err != nil {
		log.Error().Err(err).Msg(s.errContext)
	}
}
