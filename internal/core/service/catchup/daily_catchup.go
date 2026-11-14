package catchup

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/rs/zerolog/log"
)

const (
	postCloseDelay = 5 * time.Minute
	// marketCloseHourET es el cierre del POST-MARKET extendido, 20:00 ET (beta necesita el cierre real con post-market).
	marketCloseHourET = 20
)

// LastMaintenanceWindowStart es el arranque de la ventana de mantenimiento
// mas reciente (cierre + postCloseDelay) -- la frontera que separa "lo
// calculado en esta ventana" de "lo calculado antes": un done_at de un
// refresh de fundamentales posterior a esta marca significa que el paso ya
// corrio tras el ultimo cierre y no hay que recalcularlo en un reinicio.
func LastMaintenanceWindowStart(now time.Time) time.Time {
	return NextMaintenanceWindowAt(now).Add(-24 * time.Hour)
}

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
// testSymbolPatterns: los simbolos de prueba que TastyTrade publica en su
// propio universo de equities (NTEST/PTEST) -- el usuario los excluyo del
// mercado nuestro; no deben entrar al Tracked ni aparecer en el screener. El
// prefijo cubre las variantes (NTEST/A, ...).
var testSymbolPatterns = []string{"NTEST", "PTEST"}

// testSymbolSuffix: la familia UTP/NASDAQ "Z..ZZT"; ninguna accion real termina en ZZT.
const testSymbolSuffix = "ZZT"

func isTestSymbol(symbol string) bool {
	if strings.HasSuffix(symbol, testSymbolSuffix) {
		return true
	}
	for _, p := range testSymbolPatterns {
		if strings.HasPrefix(symbol, p) {
			return true
		}
	}
	return false
}

func reconcileUniverse(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository) error {
	active, err := gateway.ActiveSymbols(ctx)
	if err != nil {
		return fmt.Errorf("fetching active symbol universe: %w", err)
	}
	if len(active) == 0 {
		return fmt.Errorf("active symbol universe came back empty, skipping reconciliation")
	}
	real := active[:0]
	for _, s := range active {
		if !isTestSymbol(s.Symbol) {
			real = append(real, s)
		}
	}
	if err := symbols.Upsert(ctx, real); err != nil {
		return fmt.Errorf("upserting active symbols: %w", err)
	}

	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		return fmt.Errorf("listing tracked symbols: %w", err)
	}
	// activeSet se arma desde `real` (ya filtrado): `real := active[:0]` reutiliza el mismo array de abajo.
	activeSet := make(map[string]struct{}, len(real))
	for _, s := range real {
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

// phaseJobs separa una fase en los simbolos sin datos todavia (uncovered,
// van por el probe de profundidad individual) y los que ya tienen algo
// (covered, van por lotes de una sola suscripcion DxLink) -- un chequeo con
// al menos una barra nueva cuesta lo mismo en red que traer la profundidad
// completa de un simbolo nuevo (no es proporcional a cuantas barras trae),
// asi que sin esta prioridad una fase gasta su tiempo re-chequeando
// simbolos que ya estan al dia antes de llegar siquiera a los que nunca se
// tocaron.
func phaseJobs(tracked []domain.Symbol, tf domain.Timeframe, withData map[string]struct{}) (uncovered, covered []job) {
	for _, s := range tracked {
		j := job{symbol: s.Symbol, tf: tf}
		if _, ok := withData[s.Symbol]; ok {
			covered = append(covered, j)
		} else {
			uncovered = append(uncovered, j)
		}
	}
	return uncovered, covered
}

const (
	backfillMaxAttempts = 3
	backfillRetryDelay  = 5 * time.Second
)

// sweepBatchSize es el tamano de lote del barrido -- el agrupamiento
// original del pool de Java (100 simbolos por suscripcion DxLink): con
// esto la fase D1 pasa de 13k round-trips de add/remove a ~130 mensajes.
const sweepBatchSize = 100

// runPhase corre UN timeframe hasta el final; todos los workers terminan (wg.Wait) antes de la fase siguiente.
func runPhase(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, uncovered, covered []job, tf domain.Timeframe, workers int) {
	start := time.Now()

	var (
		emptyMu        sync.Mutex
		empty          []string
		partialBatches int
		wg             sync.WaitGroup
	)
	queue := make(chan job, len(uncovered))
	for _, j := range uncovered {
		queue <- j
	}
	close(queue)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				if backfillWithRetry(ctx, ingest, j) {
					emptyMu.Lock()
					empty = append(empty, j.symbol)
					emptyMu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	batches := make(chan []job, len(covered)/sweepBatchSize+1)
	for i := 0; i < len(covered); i += sweepBatchSize {
		end := min(i+sweepBatchSize, len(covered))
		batches <- covered[i:end]
	}
	close(batches)

	duration, err := tf.Duration()
	if err != nil {
		log.Error().Err(err).Str("timeframe", string(tf)).Msg("universe sweep: resolving timeframe duration")
		return
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batches {
				batchEmpty, partial := backfillBatchWithRetry(ctx, gateway, candles, ingest, batch, tf, duration)
				if len(batchEmpty) > 0 || partial {
					emptyMu.Lock()
					if len(batchEmpty) > 0 {
						empty = append(empty, batchEmpty...)
					}
					if partial {
						partialBatches++
					}
					emptyMu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	ev := log.Info()
	if len(empty) > 0 {
		ev = log.Warn()
		sort.Strings(empty)
		sample := empty
		if len(sample) > 50 {
			sample = sample[:50]
		}
		ev = ev.Strs("emptySample", sample)
	}
	ev.Str("timeframe", string(tf)).
		Int("symbols", len(uncovered)+len(covered)).
		Int("empty", len(empty)).
		Int("partialBatches", partialBatches).
		Dur("elapsed", time.Since(start)).
		Msg("universe sweep phase finished")
	reportPhase(tf, len(uncovered)+len(covered), len(empty), partialBatches, time.Since(start))
}

// noWatermarkM1Fallback: cuanto pedir hacia atras para un simbolo M1 sin backfill real (solo velas del stream en vivo).
const noWatermarkM1Fallback = 24 * time.Hour

// backfillBatchWithRetry arma el lote: lee el watermark de cada simbolo,
// descarta los que no tienen nada nuevo cerrado todavia (HasNewClosedBar)
// y pide el resto en UNA suscripcion con el margen incremental de cada uno.
// Si el fetch del lote falla, cae al backfill individual con reintentos --
// peor lento que dejar un lote sin datos.
func backfillBatchWithRetry(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, batch []job, tf domain.Timeframe, duration time.Duration) ([]string, bool) {
	symbols := make([]string, 0, len(batch))
	for _, j := range batch {
		symbols = append(symbols, j.symbol)
	}
	watermarks, err := candles.GetWatermarksBatch(ctx, symbols, tf)
	if err != nil {
		log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(batch)).
			Msg("universe sweep watermark batch failed, falling back to per-symbol backfill")
		for _, j := range batch {
			backfillWithRetry(ctx, ingest, j)
		}
		return nil, false
	}
	froms := make(map[string]time.Time, len(batch))
	for _, j := range batch {
		newest, ok := watermarks[j.symbol]
		if ok {
			if !ingestion.HasNewClosedBar(newest, duration) {
				continue
			}
			froms[j.symbol] = newest.Add(-ingestion.IncrementalMargin * duration)
			continue
		}
		if tf == domain.M1 {
			froms[j.symbol] = time.Now().Add(-noWatermarkM1Fallback)
		}
	}
	if len(froms) == 0 {
		return nil, false
	}

	result, err := gateway.GetCandlesBatch(ctx, tf, froms)
	if err != nil {
		if errors.Is(err, domain.ErrPartialHistory) {
			log.Warn().Str("timeframe", string(tf)).Int("symbols", len(froms)).
				Msg("universe sweep batch cut by timeout, saving without watermark")
			var empty []string
			for symbol := range froms {
				if len(result[symbol]) == 0 {
					empty = append(empty, symbol)
				}
			}
			saveBatchSweep(ctx, candles, ingest, result, tf, false)
			return empty, true
		}
		log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(froms)).
			Msg("universe sweep batch fetch failed, falling back to per-symbol backfill")
		for symbol := range froms {
			backfillWithRetry(ctx, ingest, job{symbol: symbol, tf: tf})
		}
		return nil, false
	}

	var empty []string
	for symbol := range froms {
		if len(result[symbol]) == 0 {
			empty = append(empty, symbol)
		}
	}

	saveBatchSweep(ctx, candles, ingest, result, tf, true)
	return empty, false
}

// saveBatchSweep junta las velas cerradas de TODOS los simbolos del lote en un solo Save(); si falla, cae al backfill individual.
func saveBatchSweep(ctx context.Context, candles out.CandleRepository, ingest in.IngestCandlesService, result map[string][]domain.Candle, tf domain.Timeframe, withWatermark bool) {
	merged := make([]domain.Candle, 0, len(result))
	bySymbol := make(map[string]struct{}, len(result))
	for symbol, candlesFetched := range result {
		closed := domain.ClosedCandles(candlesFetched, time.Now())
		if len(closed) == 0 {
			continue
		}
		merged = append(merged, closed...)
		bySymbol[symbol] = struct{}{}
	}
	if len(merged) == 0 {
		return
	}
	if err := candles.Save(ctx, merged, withWatermark); err != nil {
		log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(bySymbol)).
			Msg("universe sweep batch save failed, falling back to per-symbol backfill")
		for symbol := range bySymbol {
			backfillWithRetry(ctx, ingest, job{symbol: symbol, tf: tf})
		}
		return
	}
	// Sin este RecordTodaysClosedCandles, el SnapshotTracker se quedaria con el volumen de ayer hasta el proximo reconcile.
	ingest.RecordTodaysClosedCandles(merged)
}

// backfillWithRetry reintenta un trabajo de backfill fallido; cada worker tiene su propia cola, asi que reintentar solo lo demora a el.
func backfillWithRetry(ctx context.Context, ingest in.IngestCandlesService, j job) bool {
	var err error
	for attempt := 1; attempt <= backfillMaxAttempts; attempt++ {
		if err = ingest.Backfill(ctx, j.symbol, j.tf); err == nil {
			return false
		}
		if attempt < backfillMaxAttempts {
			time.Sleep(backfillRetryDelay)
		}
	}
	if errors.Is(err, ingestion.ErrEmptyBackfill) {
		log.Debug().Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Int("attempts", backfillMaxAttempts).
			Msg("universe sweep job returned no candles after retries")
		return true
	}
	log.Error().Err(err).Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Int("attempts", backfillMaxAttempts).
		Msg("universe sweep job failed after retries")
	return false
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
// ReconcileAndTracked reconcilia el universo con TastyTrade y devuelve la
// lista rastreada -- separado del barrido porque el arranque en frio
// necesita la lista ANTES del barrido para reanudar el M1 en vivo de
// inmediato (ver StartUniverseCycle en cmd/api).
func ReconcileAndTracked(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository) []domain.Symbol {
	if err := reconcileUniverse(ctx, gateway, symbols); err != nil {
		log.Error().Err(err).Msg("failed to reconcile symbol universe, sweeping last known tracked list")
	}

	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list tracked symbols for universe sweep")
		return nil
	}
	return tracked
}

// RunSweepPhase ejecuta el barrido de UN timeframe y cierra TODAS las conexiones DxLink al terminar.
func RunSweepPhase(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, tracked []domain.Symbol, tf domain.Timeframe, workers int) {
	if len(tracked) == 0 {
		return
	}

	withData, err := candles.SymbolsWithData(ctx, tf)
	if err != nil {
		log.Error().Err(err).Str("timeframe", string(tf)).Msg("failed to check which symbols already have data, treating all as uncovered")
		withData = map[string]struct{}{}
	}
	uncovered, covered := phaseJobs(tracked, tf, withData)
	runPhase(ctx, gateway, candles, ingest, uncovered, covered, tf, workers)

	gateway.ResetLiveConnections()
}

// RunSweep conserva el comportamiento original de barrer D1 y H1 seguidos
// (los llamadores que no necesitan intercalar calculos entre fases).
func RunSweep(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, tracked []domain.Symbol, workers int) {
	RunSweepPhase(ctx, gateway, candles, ingest, tracked, domain.D1, workers)
	RunSweepPhase(ctx, gateway, candles, ingest, tracked, domain.H1, workers)
}
