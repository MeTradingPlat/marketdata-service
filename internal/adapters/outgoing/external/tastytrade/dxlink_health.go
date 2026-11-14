package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

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
	var disconnectedSince time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Close() marca closing=true pero no cancela ctx (de vida larga,
			// del barrido/proceso) -- sin este chequeo, esta goroutine (y el
			// DxLinkConn entero, que sigue vivo mientras algo lo referencie)
			// quedaba corriendo para siempre tras cada cierre en cada frontera
			// D1->H1->M1, llamando a scheduleReconnect solo para que se
			// auto-descarte por closing=true. Confirmado como causa de fuga
			// de memoria acumulada entre reinicios (2026-09-11).
			if c.closing.Load() {
				return
			}
			if !c.Connected() {
				if disconnectedSince.IsZero() {
					disconnectedSince = time.Now()
				} else if d := time.Since(disconnectedSince); d > 5*time.Minute {
					log.Warn().Dur("duration", d).Msg("dxlink disconnected for more than 5m while reconnecting")
				}
				c.scheduleReconnect(ctx)
				continue
			}
			disconnectedSince = time.Time{}
			if age := c.lastMessageAge(); age > staleConnectionThreshold {
				log.Warn().Dur("silence", age).Msg("dxlink connection silent too long, forcing close to trigger reconnect")
				c.dropAndReconnect(ctx)
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
			// Mismo motivo que en healthCheckLoop: Close() no cancela ctx.
			if c.closing.Load() {
				return
			}
			if c.Connected() {
				if err := c.send(keepaliveMessage{Type: "KEEPALIVE", Channel: 0}); err != nil {
					log.Error().Err(err).Msg("dxlink: failed to send keepalive")
				}
			}
		}
	}
}
