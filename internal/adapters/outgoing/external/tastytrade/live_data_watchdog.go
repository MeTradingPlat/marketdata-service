package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	liveDataWatchdogInterval = 30 * time.Second

	// liveDataSilenceThreshold: con miles de simbolos suscritos (varios de
	// ellos de alta liquidez, operando varias veces por segundo en horario
	// de mercado), un silencio TOTAL de mas de 3 minutos es imposible bajo
	// operacion normal -- confirmado en vivo el 2026-08-31: el apagon real
	// duro 3+ HORAS porque nada vigilaba esto, solo el socket (ver
	// CandlePool.lastLiveEventAtUnixNano).
	liveDataSilenceThreshold = 3 * time.Minute

	// liveDataRetriggerCooldown: ForceReconnectAll no es instantaneo (cada
	// conexion reautentica y resuscribe sus simbolos en secuencia, ver
	// handleConnectionReconnect) -- sin este colchon, un tick del watchdog
	// que cae a mitad de esa recuperacion veria el silencio TODAVIA activo y
	// dispararia otra tanda de reconexiones encima de la que ya esta en
	// curso, reiniciando el reloj de reconectAttempts de cada conexion sin
	// necesidad.
	liveDataRetriggerCooldown = 5 * time.Minute
)

// StartLiveDataWatchdog vigila que seguir "conectado" de verdad signifique
// que estan llegando velas -- healthCheckLoop (dxlink_reconnect.go) ya
// vigila el socket de cada conexion, pero un socket que sigue respondiendo
// KEEPALIVE con normalidad mientras la suscripcion de datos murio en
// silencio nunca lo activa (confirmado en vivo el 2026-08-31: 3+ horas sin
// una sola vela nueva, sin un solo error en el log). Esto es la misma idea
// pero a nivel del POOL entero y mirando el dato real, no el transporte.
func StartLiveDataWatchdog(ctx context.Context, pool *CandlePool) {
	ticker := time.NewTicker(liveDataWatchdogInterval)
	go func() {
		defer ticker.Stop()
		var lastTriggeredAt time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkLiveData(ctx, pool, &lastTriggeredAt)
			}
		}
	}()
}

func checkLiveData(ctx context.Context, pool *CandlePool, lastTriggeredAt *time.Time) {
	if !isExtendedSessionET(time.Now()) {
		return
	}
	if pool.LiveSubscribedCount() == 0 {
		return
	}
	if time.Since(*lastTriggeredAt) < liveDataRetriggerCooldown {
		return
	}
	age := pool.LastLiveEventAge()
	if age == 0 || age < liveDataSilenceThreshold {
		return
	}

	log.Error().Dur("silence", age).Int("live_symbols", pool.LiveSubscribedCount()).
		Msg("live data watchdog: no candles from any symbol in too long, forcing full pool reconnect")
	*lastTriggeredAt = time.Now()
	pool.ForceReconnectAll(ctx)
}

// isExtendedSessionET: mismo horario extendido (4am-8pm ET) que ya usa
// marketCloseHourET en daily_catchup.go -- fuera de esa ventana (noche,
// fin de semana) el silencio es esperado (StopAllLive del barrido nocturno
// incluido), no una falla.
func isExtendedSessionET(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	nowET := now.In(loc)
	if nowET.Weekday() == time.Saturday || nowET.Weekday() == time.Sunday {
		return false
	}
	hour := nowET.Hour()
	return hour >= 4 && hour < 20
}
