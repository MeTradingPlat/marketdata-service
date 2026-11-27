package ingestion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

type errSpool struct {
	pendingErr error
}

func (e errSpool) Write([]domain.Candle) error          { return nil }
func (e errSpool) Pending() ([]string, error)           { return nil, e.pendingErr }
func (e errSpool) Read(string) ([]domain.Candle, error) { return nil, nil }
func (e errSpool) Remove(string) error                  { return nil }

func TestRetryPendingSaves_PendingErrorReturnsFalse(t *testing.T) {
	svc := ingestion.NewIngestCandlesService(&fakeGateway{}, &fakeRepo{}, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache(), errSpool{pendingErr: errors.New("io")})
	if ok := svc.RetryPendingSaves(context.Background()); ok {
		t.Fatal("expected false when spool.Pending fails")
	}
}
