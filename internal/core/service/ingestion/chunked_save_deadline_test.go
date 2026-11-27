package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type stubSaveRepo struct {
	out.CandleRepository
	save func(context.Context, []domain.Candle, bool) error
}

func (r *stubSaveRepo) Save(ctx context.Context, c []domain.Candle, w bool) error {
	return r.save(ctx, c, w)
}

func TestFlushLiveSaves_Deadlines(t *testing.T) {
	t.Run("chunk timeout requeues candles", func(t *testing.T) {
		repo := &stubSaveRepo{save: func(ctx context.Context, _ []domain.Candle, _ bool) error {
			<-ctx.Done()
			return ctx.Err()
		}}
		svc := NewIngestCandlesService(nil, repo, nil, nil, nil, nil, nil).(*ingestCandlesService)
		svc.saveChunkTimeout = 20 * time.Millisecond
		svc.liveSaveBuffer.add(domain.Candle{Symbol: "AAPL"})
		svc.liveSaveBuffer.add(domain.Candle{Symbol: "MSFT"})
		if svc.FlushLiveSaves(context.Background()) || svc.retryBuffer.size() != 2 {
			t.Fatalf("expected failure and 2 pending retries, got size=%d", svc.retryBuffer.size())
		}
	})
	t.Run("parent deadline wins", func(t *testing.T) {
		var got time.Time
		repo := &stubSaveRepo{save: func(ctx context.Context, _ []domain.Candle, _ bool) error {
			got, _ = ctx.Deadline()
			<-ctx.Done()
			return ctx.Err()
		}}
		svc := NewIngestCandlesService(nil, repo, nil, nil, nil, nil, nil).(*ingestCandlesService)
		svc.liveSaveBuffer.add(domain.Candle{Symbol: "AAPL"})
		parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		want, _ := parent.Deadline()
		if svc.FlushLiveSaves(parent) || !got.Equal(want) {
			t.Fatalf("expected parent deadline %v, got %v", want, got)
		}
	})
	t.Run("default deadline is 60s", func(t *testing.T) {
		var dl time.Time
		var hasDL bool
		repo := &stubSaveRepo{save: func(ctx context.Context, _ []domain.Candle, _ bool) error {
			dl, hasDL = ctx.Deadline()
			return nil
		}}
		svc := NewIngestCandlesService(nil, repo, nil, nil, nil, nil, nil).(*ingestCandlesService)
		svc.liveSaveBuffer.add(domain.Candle{Symbol: "AAPL"})
		if !svc.FlushLiveSaves(context.Background()) || !hasDL || time.Until(dl) < 59*time.Second || time.Until(dl) > 61*time.Second {
			t.Fatalf("expected ~60s deadline, got hasDL=%v dl=%v", hasDL, dl)
		}
	})
}
