package ingestion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func TestBackfill(t *testing.T) {
	tests := []struct {
		name        string
		probeResult []domain.Candle
		probeErr    error
		wantSaves   int
		wantErr     bool
		wantEmpty   bool
	}{
		{
			name:        "saves candles returned by the gateway",
			probeResult: []domain.Candle{{Symbol: "AAPL", Timeframe: domain.D1, Open: 1, High: 1, Low: 1, Close: 1}},
			wantSaves:   1,
		},
		{
			name:      "no candles from gateway does not call save",
			wantSaves: 0,
			wantEmpty: true,
		},
		{
			name:     "gateway error propagates",
			probeErr: errors.New("boom"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeGateway{probeResult: tt.probeResult, probeErr: tt.probeErr}
			repo := &fakeRepo{}
			svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

			err := svc.Backfill(context.Background(), "AAPL", domain.D1)

			if tt.wantEmpty {
				if !errors.Is(err, ingestion.ErrEmptyBackfill) {
					t.Fatalf("expected ErrEmptyBackfill, got %v", err)
				}
			} else if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.saved) != tt.wantSaves {
				t.Fatalf("Save called %d times, want %d", len(repo.saved), tt.wantSaves)
			}
		})
	}
}

// TestBackfill_M1FeedsSnapshotTracker -- sin RecordTodaysClosedCandles, el
// catch-up de arranque (que guarda M1 sin pasar por el stream en vivo) le
// deja el volumen de ayer al tracker hasta el proximo reconcile contra la
// BD (hasta 20 min, confirmado en vivo el 2026-09-08 con SMH rankeando por
// encima de NVDA/TSLL/GPRO por este mismo hueco).
func TestBackfill_M1FeedsSnapshotTracker(t *testing.T) {
	today := time.Now().Add(-2 * time.Minute)
	gw := &fakeGateway{probeResult: []domain.Candle{{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: today, Close: 5, Volume: 1234}}}
	repo := &fakeRepo{}
	tracker := intraday.NewSnapshotTracker()
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), tracker, livecandles.NewDefaultRecentCache())

	if err := svc.Backfill(context.Background(), "AAPL", domain.M1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap := tracker.SnapshotBatch([]string{"AAPL"})["AAPL"]
	if snap.PreMarketVolume+snap.DayVolume+snap.PostMarketVolume != 1234 {
		t.Fatalf("expected the backfilled M1 candle to reach the tracker, got %+v", snap)
	}
}

// TestBackfill_OldCandlesDoNotResetTracker -- una vela vieja (backfill de un
// simbolo que nunca se corrio, meses de historia) no debe pisar el dia del
// tracker: ver el comentario de domain.TodaysM1Candles sobre por que
// RecordClosedCandle resetearia el tracker ENTERO si se lo dejara pasar.
func TestBackfill_OldCandlesDoNotResetTracker(t *testing.T) {
	old := time.Now().AddDate(0, 0, -30)
	gw := &fakeGateway{probeResult: []domain.Candle{{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: old, Close: 5, Volume: 999}}}
	repo := &fakeRepo{}
	tracker := intraday.NewSnapshotTracker()
	tracker.RecordClosedCandle(domain.Candle{Symbol: "MSFT", Timeframe: domain.M1, Timestamp: time.Now(), Close: 1, Volume: 42})
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), tracker, livecandles.NewDefaultRecentCache())

	if err := svc.Backfill(context.Background(), "AAPL", domain.M1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Suma las 3 sesiones en vez de solo DayVolume -- a que sesion cae
	// time.Now() (pre/regular/post) depende de la hora real a la que corra
	// el test, no es parte de lo que este test verifica.
	snap := tracker.SnapshotBatch([]string{"MSFT"})["MSFT"]
	if got := snap.PreMarketVolume + snap.DayVolume + snap.PostMarketVolume; got != 42 {
		t.Fatalf("expected MSFT's today volume to survive AAPL's old backfill, got %+v", snap)
	}
	if got := tracker.SnapshotBatch([]string{"AAPL"})["AAPL"]; got.DayVolume+got.PreMarketVolume+got.PostMarketVolume != 0 {
		t.Fatalf("expected the old AAPL candle to be filtered out, got %+v", got)
	}
}

func TestStreamLive(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw.subscribed != "AAPL" {
		t.Fatalf("subscribed to %q, want AAPL", gw.subscribed)
	}

	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 1, High: 1, Low: 1, Close: 1})

	if len(repo.saved) != 0 {
		t.Fatalf("Save should not run until FlushLiveSaves, got %d calls", len(repo.saved))
	}
	if ok := svc.FlushLiveSaves(context.Background()); !ok {
		t.Fatal("FlushLiveSaves returned false unexpectedly")
	}
	if len(repo.saved) != 1 {
		t.Fatalf("Save called %d times after flush, want 1", len(repo.saved))
	}
}

func TestStreamLive_BuffersFailedSaveForRetry(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{saveErr: errors.New("db down")}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 1, High: 1, Low: 1, Close: 1})
	if ok := svc.FlushLiveSaves(context.Background()); ok {
		t.Fatal("expected FlushLiveSaves to report failure")
	}
	if len(repo.saved) != 0 {
		t.Fatalf("Save should have failed and not recorded a save, got %d", len(repo.saved))
	}

	repo.saveErr = nil
	svc.RetryPendingSaves(context.Background())

	if len(repo.saved) != 1 {
		t.Fatalf("expected the buffered candle to be saved on retry, got %d saves", len(repo.saved))
	}
	if len(repo.saved[0]) != 1 || repo.saved[0][0].Symbol != "AAPL" {
		t.Fatalf("unexpected saved candle: %+v", repo.saved)
	}

	repo.saved = nil
	svc.RetryPendingSaves(context.Background())
	if len(repo.saved) != 0 {
		t.Fatalf("expected empty buffer to result in no further Save call, got %d", len(repo.saved))
	}
}

func TestFlushLiveSaves_BatchesMultipleClosedCandles(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 1, High: 1, Low: 1, Close: 1})
	gw.onCandle(domain.Candle{Symbol: "MSFT", Timeframe: domain.M1, Open: 2, High: 2, Low: 2, Close: 2})

	if len(repo.saved) != 0 {
		t.Fatalf("Save should not run until FlushLiveSaves, got %d calls", len(repo.saved))
	}
	svc.FlushLiveSaves(context.Background())

	if len(repo.saved) != 1 {
		t.Fatalf("expected exactly one Save call batching both candles, got %d calls", len(repo.saved))
	}
	if len(repo.saved[0]) != 2 {
		t.Fatalf("expected the single batch to contain 2 candles, got %d", len(repo.saved[0]))
	}
}

func TestFlushLiveSaves_NothingPendingDoesNotCallSave(t *testing.T) {
	repo := &fakeRepo{}
	svc := ingestion.NewIngestCandlesService(&fakeGateway{}, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if ok := svc.FlushLiveSaves(context.Background()); !ok {
		t.Fatal("expected true when nothing is pending")
	}
	if len(repo.saved) != 0 {
		t.Fatalf("expected no Save call, got %d", len(repo.saved))
	}
}

func TestFlushLiveSaves_Chunking(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 12001; i++ {
		gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: float64(i), Close: float64(i)})
	}

	if ok := svc.FlushLiveSaves(context.Background()); !ok {
		t.Fatal("FlushLiveSaves failed")
	}

	if len(repo.saved) != 3 {
		t.Fatalf("expected 3 Save calls, got %d", len(repo.saved))
	}
	if len(repo.saved[0]) != 5000 {
		t.Fatalf("expected chunk 0 size 5000, got %d", len(repo.saved[0]))
	}
	if len(repo.saved[1]) != 5000 {
		t.Fatalf("expected chunk 1 size 5000, got %d", len(repo.saved[1]))
	}
	if len(repo.saved[2]) != 2001 {
		t.Fatalf("expected chunk 2 size 2001, got %d", len(repo.saved[2]))
	}
}

func TestFlushLiveSaves_FailsOnSecondChunk(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{failOnCall: 2}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 12001; i++ {
		gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: float64(i), Close: float64(i)})
	}

	if ok := svc.FlushLiveSaves(context.Background()); ok {
		t.Fatal("expected FlushLiveSaves to fail on second chunk")
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 successful Save call before failure, got %d", len(repo.saved))
	}
	if len(repo.saved[0]) != 5000 {
		t.Fatalf("expected first chunk size 5000, got %d", len(repo.saved[0]))
	}

	if pending := svc.PendingSaves(); pending != 7001 {
		t.Fatalf("expected PendingSaves == 7001, got %d", pending)
	}
}

func TestPendingSaves_CountsLiveAndRetry(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{saveErr: errors.New("db down")}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 1, Close: 1})
	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 2, Close: 2})

	if svc.PendingSaves() != 2 {
		t.Fatalf("expected PendingSaves == 2 in live buffer, got %d", svc.PendingSaves())
	}

	svc.FlushLiveSaves(context.Background())

	if svc.PendingSaves() != 2 {
		t.Fatalf("expected PendingSaves == 2 in retry buffer, got %d", svc.PendingSaves())
	}

	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 3, Close: 3})
	if svc.PendingSaves() != 3 {
		t.Fatalf("expected PendingSaves == 3 (1 live + 2 retry), got %d", svc.PendingSaves())
	}
}

func TestBackfill_M1LiveSubscribedSkipsFetch(t *testing.T) {
	watermark := time.Now().Add(-2 * time.Hour)
	gw := &fakeGateway{
		live: map[string]bool{"AAPL": true},
	}
	repo := &fakeRepo{watermark: &watermark}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	if err := svc.Backfill(context.Background(), "AAPL", domain.M1); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gw.getCandlesCalls != 0 {
		t.Fatalf("expected GetCandles not called, got %d calls", gw.getCandlesCalls)
	}
	if len(repo.saved) != 0 {
		t.Fatalf("expected Save not called, got %d calls", len(repo.saved))
	}
}

func TestBackfill_PartialHistorySavesWithoutWatermark(t *testing.T) {
	watermark := time.Now().Add(-2 * time.Hour)
	candle := domain.Candle{Symbol: "AAPL", Timeframe: domain.H1, Timestamp: time.Now().Add(-90 * time.Minute), Open: 10, High: 12, Low: 9, Close: 11, Volume: 500}
	gw := &fakeGateway{
		getCandlesRes: []domain.Candle{candle},
		getCandlesErr: domain.ErrPartialHistory,
	}
	repo := &fakeRepo{watermark: &watermark}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache())

	err := svc.Backfill(context.Background(), "AAPL", domain.H1)
	if !errors.Is(err, domain.ErrPartialHistory) {
		t.Fatalf("expected error wrapping domain.ErrPartialHistory, got %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 Save call, got %d", len(repo.saved))
	}
	if len(repo.savedWithWatermark) != 1 || repo.savedWithWatermark[0] != false {
		t.Fatalf("expected Save with withWatermark=false, got %v", repo.savedWithWatermark)
	}
}

