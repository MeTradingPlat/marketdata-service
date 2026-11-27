package tastytrade

import (
	"context"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"
)

var reconnectStepTimeout = 3 * time.Minute

// reconnectResubscribeTimeout: la resubscripcion no puede colgar del timeout
// del handshake porque al expirar cerraba una sesion sana y reiniciaba el
// ciclo reconexion->timeout->reconexion.
var reconnectResubscribeTimeout = 10 * time.Minute

const (
	initialReconnectDelay = 5 * time.Second
	maxReconnectDelay     = 300 * time.Second
	backoffGrowthCeiling  = 10
	healthCheckInterval   = 20 * time.Second
	keepaliveInterval     = 30 * time.Second

	// sessionSaturatedJitter: sin esto, varias conexiones que se saturaron
	// juntas en el storm original vuelven a reintentar casi en el mismo
	// instante 15 min despues (su propio reloj fijo, sin relacion con el
	// jitter de 3s de SessionBreaker.Wait -- ese se comparte entre TODAS las
	// conexiones y arranca desde el mismo evento, asi que tampoco las separa
	// de verdad). Confirmado en vivo el 2026-09-16: un grupo de ~3 conexiones
	// repitiendo el mismo choque cada 15 min de forma sostenida, en vez de
	// resolverse solo. Un margen de varios minutos (no segundos) es lo que
	// hace falta para que no todas caigan en la misma ventana de nuevo.
	sessionSaturatedJitter = 3 * time.Minute

	// staleConnectionThreshold: si no llega NINGUN mensaje del servidor
	// (dato real o su propio KEEPALIVE) en este tiempo, se asume conexion
	// zombie y se cierra a la fuerza -- 3x keepaliveInterval, margen sobre
	// jitter normal sin tardar minutos en reaccionar a un silencio real.
	staleConnectionThreshold = 90 * time.Second
)

// ForceReconnect cierra la conexion y arranca una reconexion nueva sin
// esperar a que nada la detecte como caida -- mismo camino que usa
// healthCheckLoop cuando SI detecta un silencio a nivel de socket
// (cleanup+scheduleReconnect), expuesto aca para que LiveDataWatchdog pueda
// dispararlo por un motivo distinto: silencio a nivel de DATO, no de
// mensaje (ver CandlePool.ForceReconnectAll). cleanup() ya es seguro de
// llamar sobre una conexion que ya esta reconectando o caida.
func (c *DxLinkConn) ForceReconnect(ctx context.Context) {
	c.dropAndReconnect(ctx)
}

func (c *DxLinkConn) dropAndReconnect(ctx context.Context) {
	c.cleanup()
	c.scheduleReconnect(ctx)
}

func (c *DxLinkConn) handleDisconnect(ctx context.Context) {
	c.mu.Lock()
	c.setAuthenticated(false)
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
		if c.sessionSaturated.Load() && delay < sessionSaturationCooldown {
			delay = sessionSaturationCooldown + time.Duration(rand.Int63n(int64(sessionSaturatedJitter)))
		}
		select {
		case <-time.After(delay):
		case <-c.closedCh:
			return
		case <-ctx.Done():
			return
		}
		if c.closing.Load() {
			return
		}
		if err := c.performReconnect(ctx); err != nil {
			log.Error().Err(err).Int64("connID", c.id).Msg("dxlink reconnect failed")
			continue
		}
		return
	}
}
