package tastytrade

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"
)

// sessionSaturationCooldown: misma ventana que ya usaba el reconnectLoop
// individual (sessionSaturatedDelay en dxlink_reconnect.go), pero aca se
// comparte entre TODAS las conexiones del proceso.
const sessionSaturationCooldown = 15 * time.Minute

// sessionCooldownJitter separa a las conexiones que esperaban la MISMA
// ventana compartida para que no disquen todas en el mismo instante al
// vencer -- el mismo problema de fondo que connectionStaggerDelay ya
// resuelve para el crecimiento normal del pool (ver channel_allocator.go),
// aplicado aca al reintento masivo tras un storm.
const sessionCooldownJitter = 3 * time.Second

// SessionBreaker coordina el rechazo por limite de sesiones entre TODAS las
// conexiones DxLink del proceso -- antes, cada DxLinkConn llevaba su propia
// bandera sessionSaturated y esperaba 15 min por su cuenta, pero eso solo
// protegia a una conexion que YA estaba autenticada y recibio un ERROR de
// protocolo. Las conexiones nuevas que el barrido nocturno abre bajo demanda
// (backfillWithRetry -> channelAllocator.allocate -> connFactory) nunca
// llegan a autenticarse cuando el limite esta saturado: el servidor cierra
// el handshake de entrada (close 1000 "Bye", sin ERROR de protocolo), asi
// que esa bandera individual nunca se activaba para ellas y el llamador
// reintentaba a los 5s sin ninguna conciencia del limite -- decenas de
// workers reabriendo conexiones sin parar es justo lo que le impide a las
// sesiones huerfanas de TastyTrade drenar y liberar el limite. Confirmado
// en vivo el 2026-08-30/31: storm sostenido 30+ min, 500-1000 errores/min
// sin bajar, mientras el barrido seguia pidiendo conexiones nuevas cada
// pocos segundos en paralelo con los reconnects del pool en vivo.
type SessionBreaker struct {
	cooldownUntilUnixNano atomic.Int64
}

// MarkSaturated abre la ventana compartida -- se llama tanto desde un
// ERROR:UNAUTHORIZED de una conexion ya autenticada como desde un intento de
// conexion nuevo que el servidor cierra de entrada durante el handshake.
//
// No la extiende si ya hay una ventana activa: durante un storm real, decenas
// de conexiones detectan el mismo rechazo en los minutos siguientes a la
// primera, no todas en el mismo instante -- si cada deteccion reiniciara el
// reloj a "ahora + 15 min", la ventana se corre hacia adelante sin parar
// mientras el storm sigue en curso, y la espera real termina siendo mucho
// mas larga que los 15 min pensados (confirmado en vivo el 2026-09-02: un
// storm que debia resolverse en 15 min tardo ~24). Solo la PRIMERA deteccion
// de cada storm fija el reloj; el resto lo respeta.
func (b *SessionBreaker) MarkSaturated() {
	newDeadline := time.Now().Add(sessionSaturationCooldown).UnixNano()
	for {
		current := b.cooldownUntilUnixNano.Load()
		if current > time.Now().UnixNano() {
			return
		}
		if b.cooldownUntilUnixNano.CompareAndSwap(current, newDeadline) {
			return
		}
	}
}

// Wait bloquea hasta que la ventana compartida termine (si esta activa) mas
// un jitter aleatorio, o hasta que ctx se cancele. Se llama ANTES de
// cualquier intento de dial -- en vivo o del barrido -- para que ningun
// llamador gaste un intento mas mientras el limite sigue saturado.
func (b *SessionBreaker) Wait(ctx context.Context) error {
	until := b.cooldownUntilUnixNano.Load()
	if until == 0 {
		return nil
	}
	wait := time.Until(time.Unix(0, until))
	if wait <= 0 {
		return nil
	}
	wait += time.Duration(rand.Int63n(int64(sessionCooldownJitter)))
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
