package ingestion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/spool"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func TestSpoolPendingSaves_RecoversAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	sp1 := spool.NewFileSpool(dir, 1<<20)
	gw := &fakeGateway{}
	repo1 := &fakeRepo{saveErr: errors.New("db down")}
	svc1 := ingestion.NewIngestCandlesService(gw, repo1, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache(), sp1)
	_ = svc1.StreamLive(context.Background(), "AAPL")
	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Close: 1})
	svc1.FlushLiveSaves(context.Background())
	gw.onCandle(domain.Candle{Symbol: "MSFT", Timeframe: domain.M1, Close: 2})
	svc1.(interface{ SpoolPendingSaves() }).SpoolPendingSaves()
	if svc1.PendingSaves() != 0 {
		t.Fatalf("PendingSaves = %d, want 0", svc1.PendingSaves())
	}
	pending, _ := sp1.Pending()
	if len(pending) == 0 {
		t.Fatal("expected candles written to spool")
	}
	sp2 := spool.NewFileSpool(dir, 1<<20)
	repo2 := &fakeRepo{}
	svc2 := ingestion.NewIngestCandlesService(&fakeGateway{}, repo2, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache(), sp2)
	if !svc2.RetryPendingSaves(context.Background()) {
		t.Fatal("expected second service to recover spooled candles")
	}
	if len(repo2.saved) != 1 || len(repo2.saved[0]) != 2 {
		t.Fatalf("saved = %+v, want 2 candles", repo2.saved)
	}
}
