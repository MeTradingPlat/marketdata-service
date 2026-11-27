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

func TestRetryPendingSaves_SkipsFailingSpoolAndCapsConsecutiveFailures(t *testing.T) {
	t.Run("oldest failing file does not block newer files", func(t *testing.T) {
		sp := spool.NewFileSpool(t.TempDir(), 1<<20)
		_ = sp.Write([]domain.Candle{{Symbol: "BAD", Timeframe: domain.M1, Close: 1}})
		_ = sp.Write([]domain.Candle{{Symbol: "AAPL", Timeframe: domain.M1, Close: 2}})
		_ = sp.Write([]domain.Candle{{Symbol: "MSFT", Timeframe: domain.M1, Close: 3}})
		var saved []string
		repo := &fakeRepo{saveFn: func(_ context.Context, c []domain.Candle, _ bool) error {
			if c[0].Symbol == "BAD" {
				return errors.New("bad batch")
			}
			saved = append(saved, c[0].Symbol)
			return nil
		}}
		svc := ingestion.NewIngestCandlesService(&fakeGateway{}, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache(), sp)
		if svc.RetryPendingSaves(context.Background()) {
			t.Fatal("expected false when oldest batch failed")
		}
		if len(saved) != 2 || saved[0] != "AAPL" || saved[1] != "MSFT" {
			t.Fatalf("saved = %v, want [AAPL MSFT]", saved)
		}
		pending, _ := sp.Pending()
		if len(pending) != 1 {
			t.Fatalf("pending = %d, want 1", len(pending))
		}
	})
	t.Run("stops after 3 consecutive spool failures even if memory retry fails", func(t *testing.T) {
		sp := spool.NewFileSpool(t.TempDir(), 1<<20)
		for range 5 {
			_ = sp.Write([]domain.Candle{{Symbol: "AAPL", Timeframe: domain.M1, Close: 1}})
		}
		gw := &fakeGateway{}
		repo := &fakeRepo{saveErr: errors.New("db down")}
		svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache(), sp)
		_ = svc.StreamLive(context.Background(), "AAPL")
		gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Close: 1})
		svc.FlushLiveSaves(context.Background())
		repo.saveCalls = 0
		if svc.RetryPendingSaves(context.Background()) {
			t.Fatal("expected false when db is down")
		}
		if repo.saveCalls != 4 {
			t.Fatalf("saveCalls = %d, want 4 (1 memory + 3 spool)", repo.saveCalls)
		}
		pending, _ := sp.Pending()
		if len(pending) != 5 {
			t.Fatalf("pending = %d, want 5", len(pending))
		}
	})
}
