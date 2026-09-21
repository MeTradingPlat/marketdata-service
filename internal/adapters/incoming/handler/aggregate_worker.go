package handler

import (
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

var aggregateCloseDelay = 3 * time.Second

type aggregateWorker struct {
	mu         sync.Mutex
	cur        *periodState
	closed     *periodState
	tf         domain.Timeframe
	out        *livecandles.Broadcaster[dto.CandleBar]
	stopRaw    func()
	refCount   int
	lastClosed int64
	timer      *time.Timer
}

func newAggregateWorker(tf domain.Timeframe, seed *dto.CandleBar) *aggregateWorker {
	w := &aggregateWorker{out: livecandles.NewBroadcaster[dto.CandleBar](), tf: tf}
	if seed != nil {
		w.cur = seededPeriodState(*seed, time.Now().UTC().Truncate(time.Minute).Unix())
		w.mu.Lock()
		w.armLocked()
		w.mu.Unlock()
	}
	return w
}

func (w *aggregateWorker) onTick(c domain.Candle) {
	if c.Close == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	period := livecandles.FormingPeriodStart(c.Timestamp, w.tf).Unix()
	switch {
	case w.closed != nil && period == w.closed.bar.Time:
		w.correctClosedLocked(c)
	case period <= w.lastClosed || (w.cur != nil && period < w.cur.bar.Time):
	case w.cur == nil || w.cur.bar.Time != period:
		w.closeLocked()
		w.cur = newPeriodState(period, c)
		w.armLocked()
		w.out.Publish(aggregateBroadcastKey, w.cur.bar)
	default:
		w.cur.apply(c)
		w.out.Publish(aggregateBroadcastKey, w.cur.bar)
	}
}

func (w *aggregateWorker) correctClosedLocked(c domain.Candle) {
	before := w.closed.bar
	w.closed.apply(c)
	if w.closed.bar == before {
		return
	}
	corrected := w.closed.bar
	corrected.Corrected = true
	w.out.Publish(aggregateBroadcastKey, corrected)
}

func (w *aggregateWorker) closeLocked() {
	if w.timer != nil {
		w.timer.Stop()
	}
	if w.cur == nil {
		return
	}
	w.cur.bar.Closed = true
	w.closed = w.cur
	w.lastClosed = w.cur.bar.Time
	w.cur = nil
	w.out.Publish(aggregateBroadcastKey, w.closed.bar)
}

func (w *aggregateWorker) armLocked() {
	if w.timer != nil {
		w.timer.Stop()
	}
	period := w.cur.bar.Time
	end := livecandles.NextPeriodStart(time.Unix(period, 0).UTC(), w.tf)
	w.timer = time.AfterFunc(time.Until(end)+aggregateCloseDelay, func() { w.closeElapsed(period) })
}

func (w *aggregateWorker) closeElapsed(period int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cur != nil && w.cur.bar.Time == period {
		w.closeLocked()
	}
}

func (w *aggregateWorker) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
}
