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
type baseWSSession struct {
	conn    *websocket.Conn
	writeMu sync.Mutex

	mu   sync.Mutex
	subs map[string]func()

	// errContext identifica la sesion en el log de un sendJSON fallido --
	// unico dato que de verdad difiere entre wsSession y relayWSSession[T]
	// en esta parte compartida.
	errContext string
}

func newBaseWSSession(conn *websocket.Conn, errContext string) baseWSSession {
	return baseWSSession{conn: conn, subs: make(map[string]func()), errContext: errContext}
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
	s.conn.Close()
}

func (s *baseWSSession) sendJSON(v any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.WriteJSON(v); err != nil {
		log.Error().Err(err).Msg(s.errContext)
	}
}
