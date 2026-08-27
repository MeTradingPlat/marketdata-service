package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	initialReconnectDelay = 5 * time.Second
	maxReconnectDelay     = 300 * time.Second
	backoffGrowthCeiling  = 10
	healthCheckInterval   = 20 * time.Second
	keepaliveInterval     = 30 * time.Second

	// sessionSaturatedDelay: cuando el servidor rechaza por limite de
	// sesiones, el backoff normal no basta -- cada handshake rechazado deja
	// una sesion que tarda mas en expirar que el intervalo entre intentos
	// (5 min max), asi el storm se auto-alimenta indefinidamente (confirmado
	// en vivo el 2026-08-18: saturado 17:16 -> 18:54 sin resolver). Con 15
	// min entre intentos las sesiones expiran entre medio y la mesa se
	// desocupa de verdad.
	sessionSaturatedDelay = 15 * time.Minute

	// staleConnectionThreshold: si no llega NINGUN mensaje del servidor
	// (dato real o su propio KEEPALIVE) en este tiempo, se asume conexion
	// zombie y se cierra a la fuerza -- 3x keepaliveInterval, margen sobre
	// jitter normal sin tardar minutos en reaccionar a un silencio real.
	staleConnectionThreshold = 90 * time.Second
)

func (c *DxLinkConn) handleDisconnect(ctx context.Context) {
	c.mu.Lock()
	c.authenticated = false
	c.mu.Unlock()
	c.scheduleReconnect(ctx)
}

// scheduleReconnect arranca el bucle de reintentos si no hay uno ya
// corriendo. Antes, un intento fallido programaba el siguiente llamandose a
// si misma DESDE DENTRO de la goroutine que todavia tenia reconnecting=true
// -- esa llamada se auto-descartaba en silencio (el guard de arriba la veia
// como "ya hay una reconexion en curso"), dejando la conexion muerta para
// siempre sin ningun rastro en el log. Confirmado en vivo el 2026-08-27:
// ~40 canales quedaron mudos 2 horas en pleno mercado abierto por esto.
// reconnectLoop reemplaza esa recursion con un bucle dentro de UNA sola
// goroutine que nunca suelta la bandera a mitad de camino.
func (c *DxLinkConn) scheduleReconnect(ctx context.Context) {
	if c.closing.Load() {
		return
	}
	c.reconnectMu.Lock()
	if c.reconnecting {
		c.reconnectMu.Unlock()
		return
	}
	c.reconnecting = true
	c.reconnectMu.Unlock()

	if c.Connected() {
		c.reconnectMu.Lock()
		c.reconnecting = false
		c.reconnectMu.Unlock()
		return
	}

	go c.reconnectLoop(ctx)
}

func reconnectDelay(attempts int) time.Duration {
	if attempts > backoffGrowthCeiling {
		return maxReconnectDelay
	}
	delay := initialReconnectDelay * time.Duration(int64(1)<<uint(attempts-1))
	if delay > maxReconnectDelay {
		return maxReconnectDelay
	}
	return delay
}

// reconnectLoop reintenta hasta conectar o hasta que ctx se cancele -- nunca
// se da por vencido (una racha larga de fallos, ej. host con I/O lento,
// solo alarga el delay hasta el techo) y nunca resetea reconnecting a mitad
// de camino, asi un intento fallido SIEMPRE programa el siguiente dentro de
// este mismo bucle en vez de una llamada recursiva que puede perderse.
func (c *DxLinkConn) reconnectLoop(ctx context.Context) {
	defer func() {
		c.reconnectMu.Lock()
		c.reconnecting = false
		c.reconnectMu.Unlock()
	}()
	for {
		c.reconnectMu.Lock()
		c.reconnectAttempts++
		attempts := c.reconnectAttempts
		c.reconnectMu.Unlock()

		delay := reconnectDelay(int(attempts))
		if c.sessionSaturated.Load() && delay < sessionSaturatedDelay {
			delay = sessionSaturatedDelay
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		if c.closing.Load() {
			return
		}
		if err := c.performReconnect(ctx); err != nil {
			log.Error().Err(err).Msg("dxlink reconnect failed")
			continue
		}
		return
	}
}

func (c *DxLinkConn) performReconnect(ctx context.Context) error {
	if c.Connected() {
		return nil
	}
	// Sesiones huerfanas saturando el limite: cerrarlas via REST (logout +
	// refresh del token) antes del handshake -- si el logout falla se
	// reintenta igual: el backoff a 5 min y las sesiones que expiran solas
	// siguen siendo la red de seguridad.
	if c.sessionSaturated.Swap(false) && c.onSessionReset != nil {
		if err := c.onSessionReset(ctx); err != nil {
			log.Error().Err(err).Msg("dxlink session reset failed, retrying anyway")
		}
	}
	c.cleanup()
	if err := c.Connect(ctx); err != nil {
		return err
	}
	if c.onReconnect != nil {
		c.onReconnect(ctx, c)
	}
	return nil
}

// healthCheckLoop es el vigia real de la conexion -- ver el comentario de
// lastMessageAtUnixNano en dxlink_conn.go: SetReadDeadline sobre el propio
// Read() del readLoop no demostro ser confiable, asi que este es el
// mecanismo que efectivamente detecta y corta una conexion zombie. cleanup()
// cierra el socket desde ESTA goroutine (no la del readLoop bloqueado), lo
// que garantiza que ese Read() bloqueado reviente con error y el proceso de
// reconexion arranque de verdad.
func (c *DxLinkConn) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.Connected() {
				c.scheduleReconnect(ctx)
				continue
			}
			if age := c.lastMessageAge(); age > staleConnectionThreshold {
				log.Warn().Dur("silence", age).Msg("dxlink connection silent too long, forcing close to trigger reconnect")
				c.cleanup()
				c.scheduleReconnect(ctx)
			}
		}
	}
}

func (c *DxLinkConn) keepaliveLoop(ctx context.Context) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if c.Connected() {
				if err := c.send(keepaliveMessage{Type: "KEEPALIVE", Channel: 0}); err != nil {
					log.Error().Err(err).Msg("dxlink: failed to send keepalive")
				}
			}
		}
	}
}
