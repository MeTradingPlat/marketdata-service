package ingestion

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/spool"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type adaptiveStubRepo struct {
	out.CandleRepository
	mu    sync.Mutex
	saved int
	limit int
}

func (r *adaptiveStubRepo) Save(ctx context.Context, candles []domain.Candle, _ bool) error {
	if len(candles) > r.limit {
		<-ctx.Done()
		return ctx.Err()
	}
	r.mu.Lock()
	r.saved += len(candles)
	r.mu.Unlock()
	return nil
}

func TestSaveInChunks_AdaptsSizeOnChunkTimeout(t *testing.T) {
	repo := &adaptiveStubRepo{limit: 1250}
	svc := &ingestCandlesService{
		repo:             repo,
		liveSaveBuffer:   newLiveSaveBuffer(),
		retryBuffer:      newSaveRetryBuffer(nil),
		saveChunkTimeout: 5 * time.Millisecond,
	}
	for i := range 5000 {
		svc.liveSaveBuffer.add(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Close: float64(i)})
	}
	svc.FlushLiveSaves(context.Background())
	for range 10 {
		if svc.RetryPendingSaves(context.Background()) && svc.PendingSaves() == 0 {
			break
		}
	}
	if svc.PendingSaves() != 0 || repo.saved != 5000 {
		t.Fatalf("pending=%d saved=%d, want 0 and 5000", svc.PendingSaves(), repo.saved)
	}
}

func makeTestCandles(n int) []domain.Candle {
	candles := make([]domain.Candle, n)
	for i := range n {
		candles[i] = domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Close: float64(i)}
	}
	return candles
}

func newAdaptiveTestSvc(limit int, sp *spool.FileSpool) (*ingestCandlesService, *adaptiveStubRepo) {
	repo := &adaptiveStubRepo{limit: limit}
	svc := &ingestCandlesService{
		repo:             repo,
		liveSaveBuffer:   newLiveSaveBuffer(),
		retryBuffer:      newSaveRetryBuffer(nil),
		spool:            sp,
		saveChunkTimeout: 5 * time.Millisecond,
	}
	return svc, repo
}

func TestRetryPendingSaves_CompletesLargeSpoolBatch(t *testing.T) {
	sp := spool.NewFileSpool(t.TempDir(), 1<<20)
	if err := sp.Write(makeTestCandles(5000)); err != nil {
		t.Fatal(err)
	}
	svc, repo := newAdaptiveTestSvc(1250, sp)
	for range 10 {
		if svc.RetryPendingSaves(context.Background()) {
			break
		}
	}
	pending, err := sp.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 || repo.saved != 5000 {
		t.Fatalf("pending=%d saved=%d, want 0 and 5000", len(pending), repo.saved)
	}
}

func TestRetryPendingSaves_SmallLiveFlushDoesNotUndoAdaptiveChunk(t *testing.T) {
	sp := spool.NewFileSpool(t.TempDir(), 1<<20)
	if err := sp.Write(makeTestCandles(5000)); err != nil {
		t.Fatal(err)
	}
	svc, repo := newAdaptiveTestSvc(1250, sp)
	flushed := 0
	for range 10 {
		svc.liveSaveBuffer.add(domain.Candle{Symbol: "MSFT", Timeframe: domain.M1, Close: 1})
		flushed++
		if !svc.FlushLiveSaves(context.Background()) {
			t.Fatal("expected small live flush to succeed")
		}
		if svc.RetryPendingSaves(context.Background()) {
			break
		}
	}
	pending, err := sp.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 || repo.saved != 5000+flushed {
		t.Fatalf("pending=%d saved=%d, want 0 and %d", len(pending), repo.saved, 5000+flushed)
	}
}
