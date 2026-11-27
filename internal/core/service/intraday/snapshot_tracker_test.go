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

func TestRecordClosedCandle_EarlierDayDoesNotResetOrAccumulate(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tracker := NewSnapshotTracker()
	today := time.Date(2026, 11, 27, 0, 0, 0, 0, loc)
	tracker.Seed(today, []string{"AAPL"}, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", DayVolume: 1000},
	})
	tracker.RecordClosedCandle(domain.Candle{
		Symbol:    "MSFT",
		Timestamp: time.Date(2026, 11, 26, 15, 59, 0, 0, loc),
		Volume:    500,
	})
	got := tracker.SnapshotBatch([]string{"AAPL", "MSFT"})
	if got["AAPL"].DayVolume != 1000 {
		t.Fatalf("AAPL DayVolume = %d, want 1000", got["AAPL"].DayVolume)
	}
	if _, ok := got["MSFT"]; ok {
		t.Fatalf("expected MSFT from earlier day not to be added, got %+v", got["MSFT"])
	}
}

func TestRecordClosedCandle_DeduplicatesAndUpdatesVolume(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tracker := NewSnapshotTracker()
	ts := time.Date(2026, 11, 27, 10, 0, 0, 0, loc)
	c := domain.Candle{Symbol: "AAPL", Timestamp: ts, Open: 10, High: 12, Low: 9, Close: 11, Volume: 100}
	tracker.RecordClosedCandle(c)
	tracker.RecordClosedCandle(c)
	c.Volume = 150
	tracker.RecordClosedCandle(c)
	snap, ok := tracker.Snapshot("AAPL")
	if !ok || snap.DayVolume != 150 {
		t.Fatalf("DayVolume = %d (ok=%v), want 150", snap.DayVolume, ok)
	}
}

func TestRecordClosedCandle_OutOfOrderNewestToOldest(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tracker := NewSnapshotTracker()
	candles := []domain.Candle{
		{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 16, 5, 0, 0, loc), Close: 106, Volume: 40},
		{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 16, 1, 0, 0, loc), Close: 105, Volume: 30},
		{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 9, 35, 0, 0, loc), Open: 102, High: 104, Low: 101, Volume: 20},
		{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 9, 30, 0, 0, loc), Open: 100, High: 103, Low: 99, Volume: 10},
		{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 8, 5, 0, 0, loc), Close: 98, Volume: 15},
		{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 8, 1, 0, 0, loc), Close: 97, Volume: 5},
	}
	for _, c := range candles {
		tracker.RecordClosedCandle(c)
	}
	snap, _ := tracker.Snapshot("AAPL")
	if snap.Open != 100 || snap.PreMarketVolume != 20 || snap.PreMarketClose != 98 ||
		snap.DayVolume != 30 || snap.PostMarketVolume != 70 || snap.PostMarketClose != 106 {
		t.Fatalf("unexpected out-of-order snapshot: %+v", snap)
	}
}

func TestRecordClosedCandle_RespectsCountedThrough(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tracker := NewSnapshotTracker()
	day := time.Date(2026, 11, 27, 0, 0, 0, 0, loc)
	tracker.Seed(day, []string{"AAPL"}, map[string]domain.IntradaySnapshot{
		"AAPL": {Symbol: "AAPL", Open: 100, DayVolume: 500, CountedThrough: time.Date(2026, 11, 27, 10, 0, 0, 0, loc)},
	})
	tracker.RecordClosedCandle(domain.Candle{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 10, 0, 0, 0, loc), Open: 99, Volume: 200})
	tracker.RecordClosedCandle(domain.Candle{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 10, 1, 0, 0, loc), Open: 101, Volume: 50})
	snap, _ := tracker.Snapshot("AAPL")
	if snap.DayVolume != 550 || snap.Open != 100 {
		t.Fatalf("CountedThrough not respected: %+v", snap)
	}
}

func TestRecordClosedCandle_NextDayResetsSessions(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tracker := NewSnapshotTracker()
	tracker.RecordClosedCandle(domain.Candle{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 26, 10, 0, 0, 0, loc), Volume: 500})
	tracker.RecordClosedCandle(domain.Candle{Symbol: "MSFT", Timestamp: time.Date(2026, 11, 27, 10, 0, 0, 0, loc), Volume: 100})
	got := tracker.SnapshotBatch([]string{"AAPL", "MSFT"})
	if _, ok := got["AAPL"]; ok {
		t.Fatalf("expected AAPL cleared on next day, got %+v", got["AAPL"])
	}
	if got["MSFT"].DayVolume != 100 {
		t.Fatalf("MSFT DayVolume = %d, want 100", got["MSFT"].DayVolume)
	}
}

func TestRecordClosedCandle_LastCloseDoesNotRegress(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tracker := NewSnapshotTracker()
	tracker.SeedLastClose([]string{"AAPL"}, map[string]domain.Candle{
		"AAPL": {Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 10, 5, 0, 0, loc), Close: 150, Volume: 10},
	})
	tracker.RecordClosedCandle(domain.Candle{Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 10, 1, 0, 0, loc), Close: 140, Volume: 99})
	price, vol, ok := tracker.LastClose("AAPL")
	if !ok || price != 150 || vol != 10 {
		t.Fatalf("LastClose regressed: price=%v vol=%d ok=%v", price, vol, ok)
	}
}

func TestRecordClosedCandle_EarlyCloseAndOutsideSession(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tracker := NewSnapshotTracker()

	tracker.RecordClosedCandle(domain.Candle{
		Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 2, 0, 0, 0, loc),
		Open: 90, High: 91, Low: 89, Close: 90, Volume: 100,
	})
	tracker.RecordClosedCandle(domain.Candle{
		Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 10, 0, 0, 0, loc),
		Open: 100, High: 105, Low: 99, Close: 104, Volume: 200,
	})
	tracker.RecordClosedCandle(domain.Candle{
		Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 13, 30, 0, 0, loc),
		Open: 110, High: 120, Low: 80, Close: 112, Volume: 300,
	})
	tracker.RecordClosedCandle(domain.Candle{
		Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 17, 30, 0, 0, loc),
		Open: 113, High: 125, Low: 70, Close: 115, Volume: 400,
	})
	tracker.RecordClosedCandle(domain.Candle{
		Symbol: "AAPL", Timestamp: time.Date(2026, 11, 27, 20, 30, 0, 0, loc),
		Open: 116, High: 130, Low: 60, Close: 118, Volume: 500,
	})

	snap, ok := tracker.Snapshot("AAPL")
	if !ok {
		t.Fatal("expected snapshot for AAPL")
	}
	if snap.PreMarketVolume != 0 || snap.PreMarketClose != 0 {
		t.Fatalf("expected 02:00 candle not in pre-market, got preVol=%d preClose=%v", snap.PreMarketVolume, snap.PreMarketClose)
	}
	if snap.DayVolume != 200 || snap.Open != 100 || snap.High != 105 || snap.Low != 99 {
		t.Fatalf("unexpected regular session on early close: %+v", snap)
	}
	if snap.PostMarketVolume != 300 || snap.PostMarketClose != 112 {
		t.Fatalf("unexpected post-market on early close: %+v", snap)
	}
	price, vol, ok := tracker.LastClose("AAPL")
	if !ok || price != 118 || vol != 500 {
		t.Fatalf("LastClose = (%v, %d, %v), want (118, 500, true)", price, vol, ok)
	}
}
