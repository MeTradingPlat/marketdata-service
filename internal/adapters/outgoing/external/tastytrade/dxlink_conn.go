package tastytrade

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const handshakeTimeout = 30 * time.Second
const closeFrameTimeout = time.Second

// tcpKeepAlivePeriod: DefaultDialer no habilita keepalive TCP -- sin esto,
// un peer/ruta muerta que un dispositivo intermedio (NAT/proxy/firewall)
// tumbo en silencio (sin FIN/RST, la causa mas comun de una "conexion
// zombie") solo se detecta por nuestro propio idleReadTimeout de la capa
// de aplicacion. El keepalive TCP es una sonda del kernel, independiente
// de que nuestro codigo de WebSocket este funcionando bien -- una segunda
// via de deteccion, mas rapida y mas barata que agregar logica propia.
const tcpKeepAlivePeriod = 15 * time.Second

var dxLinkDialer = websocket.Dialer{
	HandshakeTimeout: 30 * time.Second,
	NetDialContext:   (&net.Dialer{KeepAlive: tcpKeepAlivePeriod}).DialContext,
}

type DxLinkConn struct {
	id        int64
	urlFunc   func() string
	tokenFunc func() string

	writeMu sync.Mutex

	mu            sync.RWMutex
	ctx           context.Context
	conn          *websocket.Conn
	authenticated bool
	channels      map[int]*dxLinkChannel
	nextChannelID int32
	handshakeDone chan error

	reconnectAttempts int32
	reconnecting      bool
	reconnectMu       sync.Mutex

	// lastMessageAtUnixNano: SetReadDeadline sobre el socket resulto NO ser
	// confiable en este entorno -- confirmado en vivo con dos volcados de
	// goroutines separados: el readLoop seguia bloqueado en Read() varios
	// minutos despues de vencido el deadline, sin error, sin reconexion (es
	// un problema conocido de gorilla/websocket en ciertos escenarios, no
	// algo propio de este codigo). En vez de depender de que Read() expire
	// solo, un vigia independiente (healthCheckLoop) rastrea cuando llego
	// el ultimo mensaje real y CIERRA la conexion a la fuerza si pasa
	// demasiado tiempo -- cerrar el socket desde otra goroutine SI hace que
	// un Read() bloqueado en el reviente con error, de forma garantizada.
	lastMessageAtUnixNano atomic.Int64

	onReconnect func(ctx context.Context, c *DxLinkConn)

	// sessionSaturated: el servidor rechazo el AUTH por exceder el limite de
	// sesiones del usuario -- sesiones de conexiones previas (contenedores
	// muertos, intentos del storm de reconexion) que quedaron vivas
	// server-side. La proxima reconexion debe cerrarlas via REST
	// (onSessionReset) antes de reintentar el handshake, o el ciclo de
	// "sessions exceeded" se auto-perpetua hasta que expiran solas.
	sessionSaturated atomic.Bool

	onSessionReset func(ctx context.Context) error

	// onSessionSaturated/waitForSessionCooldown: el lado compartido del
	// mismo problema que sessionSaturated -- ver session_breaker.go. Se
	// llaman en TODA conexion (en vivo o efimera del barrido), no solo en
	// la que detecto el rechazo.
	onSessionSaturated     func()
	waitForSessionCooldown func(ctx context.Context) (func(), error)

	closing        atomic.Bool
	closeOnce      sync.Once
	closedCh       chan struct{}
	watchLoopsOnce sync.Once

	stats              dxLinkSocketStats
	socketOpenedAtNano atomic.Int64
	closeReasonMu      sync.Mutex
	closeReason        string

	// connDone se cierra en cleanup() -- es la señal de "esta conexion en
	// particular ya murio" para cualquier goroutine bloqueada esperando algo
	// que solo puede llegar por ESTE socket (ver dxLinkChannel.open). Antes
	// de esto, ese select solo tenia el ctx del llamador como salida, y ese
	// ctx es de vida larga (el barrido nocturno, sin timeout propio) -- si
	// el socket moria (zombie, sessions exceeded, INVALID_MESSAGE) justo
	// mientras se abria un canal nuevo, CHANNEL_OPENED/FEED_CONFIG ya nunca
	// iban a llegar y ese worker quedaba colgado para siempre. Confirmado en
	// vivo el 2026-08-29: una tanda de reconexiones forzadas (salto de reloj
	// del host) dejo el barrido D1 completamente detenido casi 2 horas, sin
	// un solo error en el log, con CPU en reposo -- el perfil exacto de
	// varios workers bloqueados en un canal que nunca se cierra, no de un
	// crash ni de lentitud real.
	connDone chan struct{}
}

// Done devuelve la señal de muerte de la conexion ACTUAL -- si ya esta
// caida (connDone es nil), devuelve un canal ya cerrado para que un select
// no bloquee esperando una conexion que ni siquiera existe todavia.
func (c *DxLinkConn) Done() <-chan struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.connDone == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return c.connDone
}

func (c *DxLinkConn) touchLastMessage() {
	c.lastMessageAtUnixNano.Store(time.Now().UnixNano())
}

func (c *DxLinkConn) lastMessageAge() time.Duration {
	return ageSinceUnixNano(c.lastMessageAtUnixNano.Load())
}

var nextDxLinkConnID atomic.Int64

// urlFunc/tokenFunc se evaluan recien al conectar (y en cada reconexion),
// no al construir -- el DxLinkConn se arma en el wiring de arranque, antes
// de que exista un access token o un dxlink-url real (eso llega despues,
// via OAuth.RefreshAccessToken + QuoteToken.Refresh).
func NewDxLinkConn(urlFunc func() string, tokenFunc func() string) *DxLinkConn {
	return &DxLinkConn{id: nextDxLinkConnID.Add(1), urlFunc: urlFunc, tokenFunc: tokenFunc, nextChannelID: -1, closedCh: make(chan struct{})}
}

func (c *DxLinkConn) openChannelCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.channels)
}

func (c *DxLinkConn) OnReconnect(fn func(ctx context.Context, c *DxLinkConn)) {
	c.onReconnect = fn
}

// OnSessionReset registra el hook que limpia las sesiones huerfanas de
// TastyTrade (DELETE /sessions + refresh del token) antes de reconectar
// tras un rechazo por limite de sesiones.
func (c *DxLinkConn) OnSessionReset(fn func(ctx context.Context) error) {
	c.onSessionReset = fn
}

// OnSessionSaturated registra el hook que extiende la ventana compartida de
// espera (ver session_breaker.go) -- se llama en cuanto ESTA conexion
// detecta un rechazo por limite de sesiones, para que las demas (en vivo y
// del barrido) tambien esperen antes de gastar otro intento.
func (c *DxLinkConn) OnSessionSaturated(fn func()) {
	c.onSessionSaturated = fn
}

// OnWaitForSessionCooldown registra el hook que bloquea antes de dialear si
// la ventana compartida sigue activa.
func (c *DxLinkConn) OnWaitForSessionCooldown(fn func(ctx context.Context) (func(), error)) {
	c.waitForSessionCooldown = fn
}

func (c *DxLinkConn) markSessionSaturated() {
	c.sessionSaturated.Store(true)
	if c.onSessionSaturated != nil {
		c.onSessionSaturated()
	}
}

var openDxLinkSessions atomic.Int64

func (c *DxLinkConn) setAuthenticated(authenticated bool) {
	if c.authenticated == authenticated {
		return
	}
	c.authenticated = authenticated
	if authenticated {
		log.Info().Int64("connID", c.id).Int64("openSessions", openDxLinkSessions.Add(1)).Msg("dxlink session opened")
		return
	}
	log.Info().Int64("connID", c.id).Int64("openSessions", openDxLinkSessions.Add(-1)).Str("reason", c.closeReasonText()).Msg("dxlink session closed")
}

func (c *DxLinkConn) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && c.authenticated
}
