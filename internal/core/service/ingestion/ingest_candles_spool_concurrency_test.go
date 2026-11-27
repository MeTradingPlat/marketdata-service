package ingestion_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/spool"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

type raceRepo struct {
	out.CandleRepository
	mu      sync.Mutex
	fail    atomic.Bool
	symbols map[string]bool
}

func (r *raceRepo) GetWatermark(_ context.Context, _ string, _ domain.Timeframe) (*time.Time, error) {
	return nil, nil
}

func (r *raceRepo) Save(_ context.Context, candles []domain.Candle, _ bool) error {
	if r.fail.Load() {
		return errors.New("down")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range candles {
		r.symbols[c.Symbol] = true
	}
	return nil
}

func TestFlushAndRetryConcurrent_NoLoss(t *testing.T) {
	gw, repo := &fakeGateway{}, &raceRepo{symbols: make(map[string]bool)}
	repo.fail.Store(true)
	sp := spool.NewFileSpool(t.TempDir(), 10*1024*1024)
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster[domain.Candle](), livecandles.NewBroadcaster[domain.IntradaySnapshot](), intraday.NewSnapshotTracker(), livecandles.NewDefaultRecentCache(), sp)
	_ = svc.StreamLive(context.Background(), "AAPL")
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				gw.onCandle(domain.Candle{Symbol: string(rune('A'+id)) + string(rune('0'+i)), Timeframe: domain.M1, Open: 1, Close: 1})
				svc.FlushLiveSaves(context.Background())
				svc.RetryPendingSaves(context.Background())
			}
		}(g)
	}
	wg.Wait()
	repo.fail.Store(false)
	svc.FlushLiveSaves(context.Background())
	svc.RetryPendingSaves(context.Background())
	if len(repo.symbols) != 80 {
		t.Fatalf("saved %d unique candles, want 80", len(repo.symbols))
	}
}
