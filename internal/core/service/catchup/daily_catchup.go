package catchup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const (
	postCloseDelay = 5 * time.Minute
	// marketCloseHourET es el fin del post-market extendido -- despues de
	// esto no llegan ticks nuevos hasta el pre-market del dia siguiente.
	marketCloseHourET = 20
)

// NextMaintenanceWindowAt es la proxima vez que el mercado (post-market
// extendido incluido) lleva postCloseDelay sin actividad -- se calcula en
// hora de Nueva York, no UTC. Con UTC fijo la ventana caia a medianoche UTC,
// que en horario estandar (EST, invierno) son las 7pm ET: todavia dentro
// del post-market real, compitiendo con RunSweep por las mismas conexiones
// DxLink en vez de esperar a que el mercado este de verdad inactivo (ver
// CandlePool.StopAllLive).
func NextMaintenanceWindowAt(now time.Time) time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		next := midnight.Add(postCloseDelay)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}

	nowET := now.In(loc)
	target := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), marketCloseHourET, 0, 0, 0, loc).Add(postCloseDelay)
	if !target.After(nowET) {
		target = target.AddDate(0, 0, 1)
	}
	return target
}

// reconcileUniverse trae el universo activo real de TastyTrade antes de
// cada corrida -- un simbolo nuevo (IPO, resplit, etc.) entra a Tracked()
// de inmediato en vez de esperar a que alguien lo agregue a mano, y uno que
// ya no figura como activo se desactiva (no se borra ninguna vela ya
// guardada, solo deja de pedirsele mas historia). Si el fetch falla o
// vuelve vacio (fallo silencioso del lado de TastyTrade), no se toca nada
// -- mejor un universo desactualizado que desactivar todo por un fetch roto.
func reconcileUniverse(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository) error {
	active, err := gateway.ActiveSymbols(ctx)
	if err != nil {
		return fmt.Errorf("fetching active symbol universe: %w", err)
	}
	if len(active) == 0 {
		return fmt.Errorf("active symbol universe came back empty, skipping reconciliation")
	}
	if err := symbols.Upsert(ctx, active); err != nil {
		return fmt.Errorf("upserting active symbols: %w", err)
	}

	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		return fmt.Errorf("listing tracked symbols: %w", err)
	}
	activeSet := make(map[string]struct{}, len(active))
	for _, s := range active {
		activeSet[s.Symbol] = struct{}{}
	}
	var stale []string
	for _, s := range tracked {
		if _, ok := activeSet[s.Symbol]; !ok {
			stale = append(stale, s.Symbol)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	log.Info().Int("count", len(stale)).Msg("deactivating symbols no longer in the active universe")
	return symbols.Deactivate(ctx, stale)
}

type job struct {
	symbol string
	tf     domain.Timeframe
}

// phaseJobs ordena una sola fase: los simbolos sin datos primero, los que
// ya tienen algo despues -- un chequeo con al menos una barra nueva cuesta
// lo mismo en red que traer la profundidad completa de un simbolo nuevo
// (no es proporcional a cuantas barras trae), asi que sin esta prioridad
// una fase gasta su tiempo re-chequeando simbolos que ya estan al dia antes
// de llegar siquiera a los que nunca se tocaron.
func phaseJobs(tracked []domain.Symbol, tf domain.Timeframe, withData map[string]struct{}) []job {
	var uncovered, covered []job
	for _, s := range tracked {
		j := job{symbol: s.Symbol, tf: tf}
		if _, ok := withData[s.Symbol]; ok {
			covered = append(covered, j)
		} else {
			uncovered = append(uncovered, j)
		}
	}
	return append(uncovered, covered...)
}

const (
	backfillMaxAttempts = 3
	backfillRetryDelay  = 5 * time.Second
)

// runPhase corre UN timeframe hasta el final -- todos los workers drenan la
// cola y terminan (wg.Wait) antes de que el llamador pueda arrancar la
// fase siguiente. Barrera estricta a proposito: nada de H1 empieza mientras
// quede un solo D1 pendiente, ni al reves.
func runPhase(ctx context.Context, ingest in.IngestCandlesService, jobs []job, tf domain.Timeframe, workers int) {
	start := time.Now()
	queue := make(chan job, len(jobs))
	for _, j := range jobs {
		queue <- j
	}
	close(queue)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				backfillWithRetry(ctx, ingest, j)
			}
		}()
	}
	wg.Wait()

	log.Info().Str("timeframe", string(tf)).Int("symbols", len(jobs)).Dur("elapsed", time.Since(start)).
		Msg("universe sweep phase finished")
}

// backfillWithRetry reintenta un trabajo de backfill fallido -- confirmado en
// vivo: un apagon transitorio de Postgres (segundos) durante la fase D1/H1
// dejaba simbolos completos sin H1 hasta la siguiente noche, porque este
// camino nunca tuvo reintento (a diferencia de M1, ver startLiveWithRetry).
// A diferencia del callback de M1 en vivo -- compartido por cientos de
// simbolos en el mismo hilo de lectura del WebSocket, donde bloquear ahi
// afectaria a todos -- cada worker de este pool ya procesa su propia cola:
// reintentar aca solo demora a ESE worker, no a los demas.
func backfillWithRetry(ctx context.Context, ingest in.IngestCandlesService, j job) {
	var err error
	for attempt := 1; attempt <= backfillMaxAttempts; attempt++ {
		if err = ingest.Backfill(ctx, j.symbol, j.tf); err == nil {
			return
		}
		if attempt < backfillMaxAttempts {
			time.Sleep(backfillRetryDelay)
		}
	}
	log.Error().Err(err).Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Int("attempts", backfillMaxAttempts).
		Msg("universe sweep job failed after retries")
}

// RunSweep reconcilia el universo y despues corre D1 completo (fase 1) y
// H1 completo (fase 2) para todo lo rastreado, cada fase como una barrera
// estricta (ver runPhase) -- reemplaza la agregacion SQL M1->H1/D1 que se
// tenia antes: en vez de sintetizar H1/D1 nosotros mismos, le pide a
// TastyTrade las barras nativas que falten. M1 no pasa por aqui -- lo
// orquesta el llamador (arranque: primera vez; ventana de mantenimiento:
// despues de StopAllLive), ver cmd/api. Devuelve la lista de simbolos
// rastreados para que el llamador pueda resuscribir M1 sobre el mismo
// conjunto sin pedirlo de nuevo.
func RunSweep(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, ingest in.IngestCandlesService, workers int) []domain.Symbol {
	if err := reconcileUniverse(ctx, gateway, symbols); err != nil {
		log.Error().Err(err).Msg("failed to reconcile symbol universe, sweeping last known tracked list")
	}

	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list tracked symbols for universe sweep")
		return nil
	}

	withD1, err := candles.SymbolsWithData(ctx, domain.D1)
	if err != nil {
		log.Error().Err(err).Msg("failed to check which symbols already have D1 data, treating all as uncovered")
		withD1 = map[string]struct{}{}
	}
	runPhase(ctx, ingest, phaseJobs(tracked, domain.D1, withD1), domain.D1, workers)

	// Cierra todas las conexiones DxLink entre fases -- confirmado en vivo
	// que arrastrar las conexiones de D1 justo cuando H1/M1 abren varias de
	// golpe puede superar el limite de sesiones concurrentes de TastyTrade.
	// Cada fase arranca desde cero sesiones.
	gateway.ResetLiveConnections()

	withH1, err := candles.SymbolsWithData(ctx, domain.H1)
	if err != nil {
		log.Error().Err(err).Msg("failed to check which symbols already have H1 data, treating all as uncovered")
		withH1 = map[string]struct{}{}
	}
	runPhase(ctx, ingest, phaseJobs(tracked, domain.H1, withH1), domain.H1, workers)

	gateway.ResetLiveConnections()

	return tracked
}
