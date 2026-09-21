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
	m1Time     int64
	m1Volume   int64
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
		w.m1Time, w.m1Volume = c.Timestamp.Unix(), c.Volume
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
	if c.Timestamp.Unix() == w.m1Time {
		if w.m1Volume >= 0 {
			w.agg.Volume += c.Volume - w.m1Volume
		}
	} else {
		w.agg.Volume += c.Volume
		w.m1Time = c.Timestamp.Unix()
	}
	w.m1Volume = c.Volume
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
