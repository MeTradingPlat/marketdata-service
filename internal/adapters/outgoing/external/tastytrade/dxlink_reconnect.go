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

	// staleConnectionThreshold: si no llega NINGUN mensaje del servidor
	// (dato real o su propio KEEPALIVE) en este tiempo, se asume conexion
	// zombie y se fuerza reconexion -- ver lastMessageAtUnixNano en
	// dxlink_conn.go. 3x keepaliveInterval da margen a jitter normal sin
	// tardar minutos en reaccionar a un silencio real.
	staleConnectionThreshold = 90 * time.Second
)

func (c *DxLinkConn) handleDisconnect(ctx context.Context) {
	c.mu.Lock()
	c.authenticated = false
	c.mu.Unlock()
	c.scheduleReconnect(ctx)
}

// scheduleReconnect nunca se da por vencido -- una racha larga de fallos
// (ej. host con I/O lento retrasando el handshake mas alla del timeout)
// solo alarga el delay hasta el techo, nunca deja la conexion muerta para
// siempre sin reintentar.
func (c *DxLinkConn) scheduleReconnect(ctx context.Context) {
	c.reconnectMu.Lock()
	if c.reconnecting {
		c.reconnectMu.Unlock()
		return
	}
	c.reconnecting = true
	c.reconnectAttempts++
	attempts := c.reconnectAttempts
	c.reconnectMu.Unlock()

	if c.Connected() {
		c.reconnectMu.Lock()
		c.reconnecting = false
		c.reconnectMu.Unlock()
		return
	}

	delay := reconnectDelay(int(attempts))
	go func() {
		defer func() {
			c.reconnectMu.Lock()
			c.reconnecting = false
			c.reconnectMu.Unlock()
		}()
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		c.performReconnect(ctx)
	}()
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

func (c *DxLinkConn) performReconnect(ctx context.Context) {
	if c.Connected() {
		return
	}
	c.cleanup()
	if err := c.Connect(ctx); err != nil {
		log.Error().Err(err).Msg("dxlink reconnect failed")
		c.scheduleReconnect(ctx)
		return
	}
	if c.onReconnect != nil {
		c.onReconnect(ctx, c)
	}
}

// healthCheckLoop trata el silencio prolongado igual que una desconexion
// real -- scheduleReconnect (y performReconnect) se rehusan a hacer nada si
// Connected() sigue en true, que es EXACTAMENTE lo que pasa con una
// conexion zombie (TCP nunca arrojo error, authenticated nunca se apago
// solo). handleDisconnect ya apaga authenticated antes de programar la
// reconexion -- reusarlo aca evita duplicar esa logica y garantiza que el
// reintento si ocurra.
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
			} else if c.lastMessageAge() > staleConnectionThreshold {
				log.Warn().Dur("silence", c.lastMessageAge()).Msg("dxlink connection looks zombied (no messages received), forcing reconnect")
				c.handleDisconnect(ctx)
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
