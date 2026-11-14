package catchup

import (
	"context"
	"fmt"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

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
