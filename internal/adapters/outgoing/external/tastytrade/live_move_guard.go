package tastytrade

import (
	"context"
	"fmt"
	"time"
)

var errMoveInProgress = fmt.Errorf("symbol move already in progress: %w", context.Canceled)

func (p *CandlePool) moveLiveSymbolChannel(ctx context.Context, symbol string, oldCh *pooledChannel, from time.Time, epoch uint64, now time.Time) (*pooledChannel, error) {
	if _, loaded := p.movingSymbols.LoadOrStore(symbol, struct{}{}); loaded {
		return nil, errMoveInProgress
	}
	defer p.movingSymbols.Delete(symbol)
	return p.swapLiveSymbolChannel(ctx, symbol, oldCh, from, epoch, now)
}
