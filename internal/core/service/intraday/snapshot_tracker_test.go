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

	tracker.MergeReconcile(today, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", PreMarketVolume: 5000},
		"TSLA": {Symbol: "TSLA", PreMarketVolume: 1000},
	})

	// Vuelta incompleta: solo trae TSLA (AAPL "se perdio" por presion de BD).
	tracker.MergeReconcile(today, map[string]domain.IntradaySnapshot{
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

	tracker.MergeReconcile(yesterday, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", PreMarketVolume: 5000},
	})
	tracker.MergeReconcile(today, map[string]domain.IntradaySnapshot{
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
