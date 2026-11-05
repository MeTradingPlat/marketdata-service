package catchup

import (
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

var Alert = func(level, message string, metadata map[string]any) {}

func reportPhase(tf domain.Timeframe, symbols, empty, partialBatches int, elapsed time.Duration) {
	level := "INFO"
	if empty > 0 || partialBatches > 0 {
		level = "WARN"
	}
	Alert(level, fmt.Sprintf("Barrido %s terminado: %d simbolos, %d vacios, %d lotes cortados por timeout", tf, symbols, empty, partialBatches), map[string]any{
		"timeframe":     string(tf),
		"simbolos":      symbols,
		"vacios":        empty,
		"lotesCortados": partialBatches,
		"duracionSeg":   int(elapsed.Seconds()),
	})
}
