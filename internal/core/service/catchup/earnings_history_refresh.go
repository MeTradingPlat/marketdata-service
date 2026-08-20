package catchup

import (
	"context"
	"strings"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// earningsHistoryWorkers: el endpoint de earnings history es por-simbolo
// (no hay batch), asi que esto puede disparar miles de requests si no se
// acota -- un pool chico con throttling amable le da prioridad a no
// ahogar la sesion REST de TastyTrade frente al tiempo total del job.
const earningsHistoryWorkers = 4

// isQuarterEndPlaceholder detecta el cierre de trimestre que TastyTrade
// devuelve como occurred-date para miles de simbolos (placeholder, no la
// fecha real del reporte): la prediccion sobre el placeholder fabrica la
// misma fecha falsa para todos (confirmado en vivo: 3,962 simbolos con
// 06-30-2026 y 3,978 con 2026-09-29 como proximo earnings).
func isQuarterEndPlaceholder(date string) bool {
	return strings.HasSuffix(date, "-03-31") ||
		strings.HasSuffix(date, "-06-30") ||
		strings.HasSuffix(date, "-09-30") ||
		strings.HasSuffix(date, "-12-31")
}

// RefreshEarningsHistory re-chequea SOLO los simbolos cuyo
// next_earnings_date ya vencio (o nunca se busco) contra el endpoint de
// historial de earnings de TastyTrade, y con eso:
//  1. occurredDate -- la fecha del ultimo reporte real (con EPS ya
//     publicado), que /market-metrics no trae (el frontend la muestra
//     como "Last Report Date").
//  2. Prediccion del proximo earnings via la mediana del ciclo historico
//     (ver domain.LastEarningsReport) cuando /market-metrics trae una
//     fecha vieja que ya paso.
//
// Corre en el barrido nocturno, despues de RefreshMarketMetrics (que es
// quien pisa next_earnings_date con el dato fresco de TastyTrade cuando
// existe) -- el COALESCE del upsert nunca pisa una fecha vigente con una
// prediccion, y los ya al dia ni siquiera entran en el lote de este job.
func RefreshEarningsHistory(ctx context.Context, gateway out.MarketDataGateway, fundamentalsRepo out.FundamentalsRepository) error {
	stale, err := fundamentalsRepo.GetSymbolsWithStaleEarnings(ctx)
	if err != nil {
		return fmt.Errorf("selecting stale symbols: %w", err)
	}
	if len(stale) == 0 {
		return nil
	}

	start := time.Now()
	jobs := make(chan string, len(stale))
	for _, symbol := range stale {
		jobs <- symbol
	}
	close(jobs)

	var mu sync.Mutex
	var updates []domain.Fundamentals
	var wg sync.WaitGroup
	for i := 0; i < earningsHistoryWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				reports, err := gateway.EarningsReports(ctx, symbol)
				if err != nil {
					log.Warn().Err(err).Str("symbol", symbol).Msg("earnings history fetch failed")
					continue
				}
				if len(reports) == 0 {
					continue
				}
				occurred, predicted := domain.LastEarningsReport(reports)
				f := domain.Fundamentals{Symbol: symbol}
				// El placeholder de cierre de trimestre tampoco es un
				// occurred-date real (ver el comentario de mas abajo) --
				// escribirlo igual contaminaba "Last Report Date" en el
				// frontend con una fecha fabricada en vez de dejar el dato
				// real anterior (si habia uno) sin tocar, gracias al COALESCE
				// del upsert cuando se manda vacio.
				if !isQuarterEndPlaceholder(occurred) {
					f.OccurredDate = occurred
				}
				// Solo se escribe la prediccion si cae a FUTURO: una
				// prediccion en el pasado (cadencia real distinta a la
				// mediana historica, ej. MDXH reportando semi-anual)
				// pisaria la fecha de TastyTrade con un dato peor. En ese
				// caso el simbolo queda en el lote de "stale" y se
				// re-predice cada noche hasta que el reporte real aterrice
				// en el historial y la prediccion vuelva a ser futura.
				// Ademas: el endpoint de TastyTrade devuelve el CIERRE DE
				// TRIMESTRE como occurred-date para miles de simbolos
				// (placeholder, no la fecha real del reporte -- confirmado
				// en vivo: 3,962 simbolos con 06-30-2026) -- predecir sobre
				// el placeholder fabrica la misma fecha falsa para todos
				// (3,978 con 2026-09-29). Sin fecha real, no hay prediccion.
				if predicted != "" && domain.DaysUntil(predicted) > 0 && !isQuarterEndPlaceholder(occurred) {
					f.NextEarningsDate = predicted
				}
				mu.Lock()
				updates = append(updates, f)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if err := fundamentalsRepo.UpsertEarningsHistory(ctx, updates); err != nil {
		return fmt.Errorf("upserting earnings history batch: %w", err)
	}
	log.Info().Int("symbols", len(updates)).Dur("elapsed", time.Since(start)).Msg("earnings history refresh finished")
	return nil
}
