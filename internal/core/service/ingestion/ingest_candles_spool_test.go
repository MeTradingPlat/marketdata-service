package ingestion_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/spool"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func TestRetryPendingSaves_SpoolRecoveryAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	sp := spool.NewFileSpool(dir, 1024*1024)
	if err := sp.Write([]domain.Candle{{Symbol: "AAPL"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000001-000001.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sp.Write([]domain.Candle{{Symbol: "MSFT"}}); err != nil {
		t.Fatal(err)
	}

	repo := &fakeRepo{saveErr: errors.New("db down")}
	svc := ingestion.NewIngestCandlesService(&fakeGateway{}, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache(), sp)
	if ok := svc.RetryPendingSaves(context.Background()); ok {
		t.Fatal("expected false when repo fails")
	}
	if ids, _ := sp.Pending(); len(ids) != 2 {
		t.Fatalf("expected 2 valid batches preserved, got %v", ids)
	}

	repo.saveErr = nil
	if ok := svc.RetryPendingSaves(context.Background()); !ok {
		t.Fatal("expected true after recovery")
	}
	if len(repo.saved) != 2 || repo.saved[0][0].Symbol != "AAPL" || repo.saved[1][0].Symbol != "MSFT" {
		t.Fatalf("unexpected saved batches: %+v", repo.saved)
	}
	if ids, _ := sp.Pending(); len(ids) != 0 {
		t.Fatalf("expected spool empty, got %v", ids)
	}
}
