package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/alerts"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
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
		var lastSessionsAlertAt time.Time
		var lastTriggeredAt time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runWatchdogTick(ctx, pool, time.Now(), &lastSessionsAlertAt, &lastTriggeredAt)
			}
		}
	}()
}

func runWatchdogTick(ctx context.Context, pool *CandlePool, now time.Time, lastSessionsAlertAt, lastTriggeredAt *time.Time) {
	checkOpenSessions(lastSessionsAlertAt)
	checkLiveData(ctx, pool, now, lastTriggeredAt)
	if isExtendedSessionET(now) {
		pool.StartReviveSilentLive(ctx, now)
	} else {
		pool.DropExpiredRevive(now)
	}
}

func checkLiveData(ctx context.Context, pool *CandlePool, now time.Time, lastTriggeredAt *time.Time) {
	if !isExtendedSessionET(now) || sinceExtendedOpenET(now) < liveDataSilenceThreshold {
		return
	}
	if pool.LiveSubscribedCount() == 0 || (!lastTriggeredAt.IsZero() && now.Sub(*lastTriggeredAt) < liveDataRetriggerCooldown) {
		return
	}
	age := pool.LastLiveEventAge()
	if age == 0 || age < liveDataSilenceThreshold {
		return
	}

	log.Error().Dur("silence", age).Int("live_symbols", pool.LiveSubscribedCount()).
		Msg("live data watchdog: no candles from any symbol in too long, forcing full pool reconnect")
	alerts.Notify("ERROR", "Sin velas M1 en vivo durante horario de mercado; forzando reconexion", map[string]any{"silencioSeg": int(age.Seconds()), "simbolosVivos": pool.LiveSubscribedCount()})
	*lastTriggeredAt = now
	pool.ForceReconnectAll(ctx)
}

func sinceExtendedOpenET(now time.Time) time.Duration {
	nowET, ok := nyseTimeET(now)
	if !ok {
		return 0
	}
	openET := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 4, 0, 0, 0, nowET.Location())
	return nowET.Sub(openET)
}

// isExtendedSessionET: mismo horario extendido (4am-8pm ET) que ya usa
// marketCloseHourET en daily_catchup.go -- fuera de esa ventana (noche,
// fin de semana) el silencio es esperado (StopAllLive del barrido nocturno
// incluido), no una falla.
func isExtendedSessionET(now time.Time) bool {
	nowET, ok := nyseTimeET(now)
	if !ok {
		return false
	}
	closeHour := 20
	if domain.IsNYSEEarlyClose(nowET) {
		closeHour = 17
	}
	hour := nowET.Hour()
	return hour >= 4 && hour < closeHour
}
