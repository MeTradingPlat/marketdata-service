package tastytrade

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/alerts"
	"github.com/rs/zerolog/log"
)

const (
	openSessionsAlertThreshold = 30
	openSessionsAlertCooldown  = 15 * time.Minute
)

func checkOpenSessions(lastAlertAt *time.Time) {
	n := openDxLinkSessions.Load()
	if n > openSessionsAlertThreshold && time.Since(*lastAlertAt) >= openSessionsAlertCooldown {
		log.Warn().Int64("sesiones", n).Int64("umbral", openSessionsAlertThreshold).Msg("sesiones DxLink abiertas por encima del limite")
		alerts.Notify("WARN", "Sesiones DxLink abiertas por encima del limite", map[string]any{"sesiones": n, "umbral": openSessionsAlertThreshold})
		*lastAlertAt = time.Now()
	}
}
