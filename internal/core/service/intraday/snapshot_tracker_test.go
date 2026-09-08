package intraday

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// TestMergeReconcile_KeepsSymbolsMissingFromBatch reproduce el bug en vivo
// del 2026-09-08: la consulta de lote que alimenta esto puede volver
// incompleta bajo carga de Postgres (ver el comentario de MergeReconcile).
// Un Seed() con ese resultado parcial habria borrado AAPL del tracker por
// no venir en esa vuelta -- MergeReconcile no debe hacer eso.
func TestMergeReconcile_KeepsSymbolsMissingFromBatch(t *testing.T) {
	tracker := NewSnapshotTracker()
	today := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)

	tracker.MergeReconcile(today, []string{"AAPL", "TSLA"}, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", PreMarketVolume: 5000},
		"TSLA": {Symbol: "TSLA", PreMarketVolume: 1000},
	})

	// Vuelta incompleta: solo trae TSLA (AAPL "se perdio" por presion de BD).
	tracker.MergeReconcile(today, []string{"AAPL", "TSLA"}, map[string]domain.IntradaySnapshot{
		"TSLA": {Symbol: "TSLA", PreMarketVolume: 25000},
	})

	got := tracker.SnapshotBatch([]string{"AAPL", "TSLA"})
	if got["AAPL"].PreMarketVolume != 5000 {
		t.Fatalf("expected AAPL to keep its previous volume (missing from this batch), got %+v", got["AAPL"])
	}
	if got["TSLA"].PreMarketVolume != 25000 {
		t.Fatalf("expected TSLA to be updated to the new batch value, got %+v", got["TSLA"])
	}
}

// TestMergeReconcile_ResetsOnDayChange -- un dia distinto nunca debe
// arrastrar volumen de ayer, a diferencia de una vuelta incompleta del
// mismo dia (ver el test de arriba).
func TestMergeReconcile_ResetsOnDayChange(t *testing.T) {
	tracker := NewSnapshotTracker()
	yesterday := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)

	tracker.MergeReconcile(yesterday, []string{"AAPL"}, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", PreMarketVolume: 5000},
	})
	tracker.MergeReconcile(today, []string{"TSLA"}, map[string]domain.IntradaySnapshot{
		"TSLA": {Symbol: "TSLA", PreMarketVolume: 1000},
	})

	got := tracker.SnapshotBatch([]string{"AAPL", "TSLA"})
	if got["AAPL"].PreMarketVolume != 0 {
		t.Fatalf("expected AAPL's stale entry to be cleared on day change, got %+v", got["AAPL"])
	}
	if got["TSLA"].PreMarketVolume != 1000 {
		t.Fatalf("expected TSLA to hold today's value, got %+v", got["TSLA"])
	}
}

// TestMergeReconcile_FillsConfirmedZeroVolumeSymbolsSoTheyStopHittingDB --
// sin esto, un simbolo sin ninguna vela M1 hoy (real: no opero, no un
// error) nunca aparece en `snapshots` (el GROUP BY no devuelve filas para
// el), asi que GetSnapshotsBatch lo trata como "todavia no reconciliado" y
// vuelve a pagar el mismo query caro de este mismo reconcile en cada
// request de signal-processing, para siempre. Confirmado en vivo el
// 2026-09-08: mantenia /fundamentals/realtime en 50-90s mucho despues de
// que el sweep de arranque ya habia terminado.
func TestMergeReconcile_FillsConfirmedZeroVolumeSymbolsSoTheyStopHittingDB(t *testing.T) {
	tracker := NewSnapshotTracker()
	today := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)

	tracker.MergeReconcile(today, []string{"AAPL", "ZZZQ"}, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", PreMarketVolume: 5000},
	})

	got := tracker.SnapshotBatch([]string{"AAPL", "ZZZQ"})
	if _, ok := got["ZZZQ"]; !ok {
		t.Fatal("expected ZZZQ (confirmed zero volume today) to be present in the tracker, not absent")
	}
	if got["ZZZQ"].PreMarketVolume+got["ZZZQ"].DayVolume+got["ZZZQ"].PostMarketVolume != 0 {
		t.Fatalf("expected ZZZQ's placeholder to have zero volume, got %+v", got["ZZZQ"])
	}
}

// TestMergeReconcile_ZeroFillNeverOverwritesAnExistingEntry -- el relleno
// solo debe aplicar a simbolos SIN ninguna entrada previa; uno que ya tiene
// datos (reales o un vacio de una vuelta anterior) tiene que sobrevivir
// intacto a una vuelta que no lo trajo, igual que ya prueba
// TestMergeReconcile_KeepsSymbolsMissingFromBatch para datos reales.
func TestMergeReconcile_ZeroFillNeverOverwritesAnExistingEntry(t *testing.T) {
	tracker := NewSnapshotTracker()
	today := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)

	tracker.MergeReconcile(today, []string{"AAPL"}, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", PreMarketVolume: 5000},
	})
	// AAPL empieza a operar recien despues, pero esta vuelta ni siquiera lo
	// pide (fuera de este lote) -- no debe tocarlo el relleno de otro simbolo.
	tracker.MergeReconcile(today, []string{"MSFT"}, map[string]domain.IntradaySnapshot{})

	got := tracker.SnapshotBatch([]string{"AAPL"})
	if got["AAPL"].PreMarketVolume != 5000 {
		t.Fatalf("expected AAPL's real volume to survive an unrelated reconcile batch, got %+v", got["AAPL"])
	}
}
