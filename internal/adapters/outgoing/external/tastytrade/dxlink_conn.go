package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const handshakeTimeout = 30 * time.Second

// tcpKeepAlivePeriod: DefaultDialer no habilita keepalive TCP -- sin esto,
// un peer/ruta muerta que un dispositivo intermedio (NAT/proxy/firewall)
// tumbo en silencio (sin FIN/RST, la causa mas comun de una "conexion
// zombie") solo se detecta por nuestro propio idleReadTimeout de la capa
// de aplicacion. El keepalive TCP es una sonda del kernel, independiente
// de que nuestro codigo de WebSocket este funcionando bien -- una segunda
// via de deteccion, mas rapida y mas barata que agregar logica propia.
const tcpKeepAlivePeriod = 15 * time.Second

var dxLinkDialer = websocket.Dialer{
	NetDialContext: (&net.Dialer{KeepAlive: tcpKeepAlivePeriod}).DialContext,
}

type DxLinkConn struct {
	urlFunc   func() string
	tokenFunc func() string

	writeMu sync.Mutex

	mu            sync.RWMutex
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

	closing atomic.Bool
}

func (c *DxLinkConn) touchLastMessage() {
	c.lastMessageAtUnixNano.Store(time.Now().UnixNano())
}

func (c *DxLinkConn) lastMessageAge() time.Duration {
	last := c.lastMessageAtUnixNano.Load()
	if last == 0 {
		return 0
	}
	return time.Since(time.Unix(0, last))
}

// urlFunc/tokenFunc se evaluan recien al conectar (y en cada reconexion),
// no al construir -- el DxLinkConn se arma en el wiring de arranque, antes
// de que exista un access token o un dxlink-url real (eso llega despues,
// via OAuth.RefreshAccessToken + QuoteToken.Refresh).
func NewDxLinkConn(urlFunc func() string, tokenFunc func() string) *DxLinkConn {
	return &DxLinkConn{urlFunc: urlFunc, tokenFunc: tokenFunc, nextChannelID: -1}
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

func (c *DxLinkConn) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && c.authenticated
}

func (c *DxLinkConn) Connect(ctx context.Context) error {
	conn, _, err := dxLinkDialer.DialContext(ctx, c.urlFunc(), nil)
	if err != nil {
		return fmt.Errorf("dialing dxlink websocket: %w", err)
	}

	// El default de gorilla es 64KB: un mensaje mayor (el lote de perfiles
	// de 13k simbolos del refresh externo, o un FEED_DATA de un batch
	// profundo) mataba la conexion con "content length exceeded" y alimentaba
	// el storm de reconexion -- confirmado en vivo el 2026-08-18 (el refresh
	// externo de las 16:03 llego degradado por esto). 64MB da margen de
	// sobra para cualquier lote.
	conn.SetReadLimit(64 << 20)

	handshakeDone := make(chan error, 1)
	c.mu.Lock()
	c.conn = conn
	c.authenticated = false
	c.handshakeDone = handshakeDone
	c.channels = make(map[int]*dxLinkChannel)
	c.mu.Unlock()
	c.touchLastMessage()

	go c.readLoop(ctx)

	if err := c.send(setupMessage{
		Type: "SETUP", Channel: 0, Version: "0.1-go/1.0.0",
		KeepaliveTimeout: 60, AcceptKeepaliveTimeout: 60,
	}); err != nil {
		return fmt.Errorf("sending dxlink setup: %w", err)
	}

	select {
	case err := <-handshakeDone:
		if err != nil {
			return err
		}
	case <-time.After(handshakeTimeout):
		// Sin este cleanup, el socket recien abierto (AUTH nunca llego a
		// AUTHORIZED) y su readLoop quedan colgando sin ningun vigia: el
		// healthCheckLoop que detecta conexiones zombie solo arranca DESPUES
		// de un handshake exitoso (mas abajo), asi que nadie mas los cierra.
		c.cleanup()
		return fmt.Errorf("dxlink handshake timed out")
	case <-ctx.Done():
		c.cleanup()
		return ctx.Err()
	}

	go c.keepaliveLoop(ctx)
	go c.healthCheckLoop(ctx)
	c.reconnectMu.Lock()
	c.reconnectAttempts = 0
	c.reconnectMu.Unlock()
	return nil
}

// Close es un cierre INTENCIONAL -- a diferencia de cleanup() (usado por la
// deteccion de conexion zombie, que SI debe reconectar), esto marca la
// conexion para que scheduleReconnect no dispare una reconexion automatica.
// Se usa en las fronteras D1->H1->M1 del barrido para arrancar cada fase
// con cero sesiones abiertas ante TastyTrade.
func (c *DxLinkConn) Close() {
	c.closing.Store(true)
	c.cleanup()
}

func (c *DxLinkConn) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authenticated = false
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.channels = make(map[int]*dxLinkChannel)
}

// dxLinkMaxMessageBytes es el limite documentado por dxFeed para un solo
// mensaje FEED_SUBSCRIPTION (kb.dxfeed.com/en/market-data-api/dxlink.html):
// pasado esto el servidor responde INVALID_MESSAGE "content length exceeded
// 65536 bytes." -- logueamos ANTES de mandar cualquier mensaje que se
// acerque, para tener la evidencia (cuantos simbolos, que canal) la proxima
// vez que pase, en vez de reconstruirlo despues por logs indirectos.
const dxLinkMaxMessageBytes = 65536
const dxLinkWarnMessageBytes = 55000

func (c *DxLinkConn) send(v interface{}) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("dxlink connection not open")
	}
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding dxlink message: %w", err)
	}
	if len(body) >= dxLinkWarnMessageBytes {
		logOutgoingMessageSize(v, len(body))
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, body)
}

func logOutgoingMessageSize(v interface{}, bytes int) {
	event := log.Warn().Int("bytes", bytes).Int("limit", dxLinkMaxMessageBytes)
	if msg, ok := v.(feedSubscriptionMessage); ok {
		event = event.Int("channel", msg.Channel).Int("add", len(msg.Add)).Int("remove", len(msg.Remove))
	}
	event.Msg("dxlink outgoing message near or over the documented size limit")
}

func (c *DxLinkConn) OpenChannel(ctx context.Context) (*dxLinkChannel, error) {
	if !c.Connected() {
		return nil, fmt.Errorf("dxlink connection not authenticated")
	}
	c.mu.Lock()
	c.nextChannelID += 2
	id := int(c.nextChannelID)
	ch := newDxLinkChannel(id, c)
	c.channels[id] = ch
	c.mu.Unlock()

	if err := ch.open(ctx); err != nil {
		c.mu.Lock()
		delete(c.channels, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("opening dxlink channel: %w", err)
	}
	return ch, nil
}

func (c *DxLinkConn) CloseChannel(ch *dxLinkChannel) {
	_ = ch.close()
	c.mu.Lock()
	delete(c.channels, ch.id)
	c.mu.Unlock()
}

func (c *DxLinkConn) channel(id int) *dxLinkChannel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channels[id]
}
