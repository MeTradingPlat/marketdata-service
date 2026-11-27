package intraday

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (t *SnapshotTracker) applyClosedCandleLocked(c domain.Candle, marketOpen, marketClose time.Time) {
	snap := t.data[c.Symbol]
	snap.Symbol = c.Symbol
	inRegular := !c.Timestamp.Before(marketOpen) && c.Timestamp.Before(marketClose)
	if inRegular {
		applyRegularRange(&snap, c)
	}
	if !snap.CountedThrough.IsZero() && !c.Timestamp.After(snap.CountedThrough) {
		t.data[c.Symbol] = snap
		return
	}
	t.applySessionCandleLocked(&snap, c, marketOpen, marketClose)
	t.data[c.Symbol] = snap
}

func applyRegularRange(snap *domain.IntradaySnapshot, c domain.Candle) {
	if snap.High == 0 || c.High > snap.High {
		snap.High = c.High
	}
	if snap.Low == 0 || c.Low < snap.Low {
		snap.Low = c.Low
	}
}

func (t *SnapshotTracker) applySessionCandleLocked(snap *domain.IntradaySnapshot, c domain.Candle, marketOpen, marketClose time.Time) {
	min := minuteKey(c.Timestamp)
	delta := t.minutes.record(c.Symbol, min, c.Volume)
	if c.Timestamp.Before(marketOpen) {
		snap.PreMarketVolume += delta
		if !t.minutes.hasNewerInRange(c.Symbol, min, minuteKey(marketOpen)) {
			snap.PreMarketClose = c.Close
		}
		return
	}
	if c.Timestamp.Before(marketClose) {
		snap.DayVolume += delta
		if snap.Open == 0 || t.isOldestUncountedRegular(c.Symbol, min, snap.CountedThrough, marketOpen) {
			snap.Open = c.Open
		}
		return
	}
	snap.PostMarketVolume += delta
	if !t.minutes.hasNewerInRange(c.Symbol, min, 0) {
		snap.PostMarketClose = c.Close
	}
}

func (t *SnapshotTracker) isOldestUncountedRegular(symbol string, min int64, countedThrough, marketOpen time.Time) bool {
	return t.minutes.isOldestFrom(symbol, min, minuteKey(marketOpen)) &&
		(countedThrough.IsZero() || countedThrough.Before(marketOpen))
}
