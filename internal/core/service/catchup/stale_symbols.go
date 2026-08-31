package catchup

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// staleSymbolThreshold: dejar de pedir H1/M1/suscripcion en vivo a un
// simbolo cuyo D1 lleva mas de esto sin avanzar, PESE a que el barrido D1
// del universo completo (ver runUniverseCycle, corre SIEMPRE sin excepcion)
// lo reintento esta misma noche. Confirmado en vivo el 2026-08-31: 729
// simbolos con D1 congelado hasta 20 dias, la mayoria acciones corporativas
// reales (fusion de SPAC, deslistado, nota vencida) que TastyTrade sigue
// listando como "activo" en su universo -- reconcileUniverse nunca los
// desactiva porque is_active refleja ESA lista, no si dxFeed manda dato
// nuevo. 14 dias de calendario (no de bolsa, para no complicarse con
// feriados) es mucho mas que cualquier pausa real (fin de semana largo,
// halt temporal) sin dejar simbolos muertos ocupando conexiones/canales en
// vivo para siempre.
const staleSymbolThreshold = 14 * 24 * time.Hour

// FilterStaleSymbols separa el universo en "sigue vivo" (H1/M1/vivo) y "sin
// dato nuevo hace demasiado". El D1 nocturno sigue pidiendose para TODO el
// universo sin excepcion (es la unica señal barata de "¿ya volvio?" que
// hace falta) -- si un simbolo excluido vuelve a traer D1, esta misma
// funcion lo va a ver fresco la proxima vuelta y reaparece solo en la lista
// viva, sin que nadie tenga que reactivarlo a mano.
func FilterStaleSymbols(ctx context.Context, candles out.CandleRepository, tracked []domain.Symbol, now time.Time) []domain.Symbol {
	symbolNames := make([]string, len(tracked))
	for i, s := range tracked {
		symbolNames[i] = s.Symbol
	}
	watermarks, err := candles.GetWatermarksBatch(ctx, symbolNames, domain.D1)
	if err != nil {
		log.Error().Err(err).Msg("checking D1 watermarks for stale-symbol filter failed, treating everyone as live")
		return tracked
	}

	fresh := make([]domain.Symbol, 0, len(tracked))
	staleCount := 0
	for _, s := range tracked {
		last, ok := watermarks[s.Symbol]
		if !ok || now.Sub(last) <= staleSymbolThreshold {
			fresh = append(fresh, s)
			continue
		}
		staleCount++
	}
	if staleCount > 0 {
		log.Info().Int("stale", staleCount).Int("total", len(tracked)).Dur("threshold", staleSymbolThreshold).
			Msg("excluding symbols with no new D1 candle in too long from H1/M1/live rollout")
	}
	return fresh
}
