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
	agg        *dto.CandleBar
	tf         domain.Timeframe
	out        *livecandles.Broadcaster[dto.CandleBar]
	stopRaw    func()
	refCount   int
	minuteVols map[int64]int64
	seedMinute int64
	lastClosed int64
	timer      *time.Timer
}

func (w *aggregateWorker) onTick(c domain.Candle) {
	if c.Close == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	period := livecandles.FormingPeriodStart(c.Timestamp, w.tf).Unix()
	if period <= w.lastClosed || (w.agg != nil && period < w.agg.Time) {
		return
	}
	if w.agg == nil || w.agg.Time != period {
		w.closeLocked()
		w.agg = &dto.CandleBar{Time: period, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Closed: false}
		w.minuteVols = map[int64]int64{c.Timestamp.Unix(): c.Volume}
		w.seedMinute = 0
		w.armLocked()
	} else {
		w.mergeLocked(c)
	}
	w.out.Publish(aggregateBroadcastKey, *w.agg)
}

func (w *aggregateWorker) mergeLocked(c domain.Candle) {
	if c.High > w.agg.High {
		w.agg.High = c.High
	}
	if c.Low < w.agg.Low {
		w.agg.Low = c.Low
	}
	w.agg.Close = c.Close
	minute := c.Timestamp.Unix()
	previous, seen := w.minuteVols[minute]
	switch {
	case seen:
		w.agg.Volume += c.Volume - previous
	case w.seedMinute != 0 && minute <= w.seedMinute:
	default:
		w.agg.Volume += c.Volume
	}
	w.minuteVols[minute] = c.Volume
}

func (w *aggregateWorker) closeLocked() {
	if w.timer != nil {
		w.timer.Stop()
	}
	if w.agg == nil {
		return
	}
	closed := *w.agg
	closed.Closed = true
	w.lastClosed = w.agg.Time
	w.agg = nil
	w.out.Publish(aggregateBroadcastKey, closed)
}

func (w *aggregateWorker) armLocked() {
	if w.timer != nil {
		w.timer.Stop()
	}
	period := w.agg.Time
	end := livecandles.NextPeriodStart(time.Unix(period, 0).UTC(), w.tf)
	w.timer = time.AfterFunc(time.Until(end)+aggregateCloseDelay, func() { w.closeElapsed(period) })
}

func (w *aggregateWorker) closeElapsed(period int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.agg != nil && w.agg.Time == period {
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
