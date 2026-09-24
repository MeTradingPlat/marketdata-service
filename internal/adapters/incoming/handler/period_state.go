package handler

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

type periodState struct {
	bar              dto.CandleBar
	minuteVols       map[int64]int64
	closeMinute      int64
	skipUnseenUpTo   int64
	seedMinuteVolume int64
}

func newPeriodState(period int64, c domain.Candle) *periodState {
	minute := c.Timestamp.Unix()
	return &periodState{
		bar:         dto.CandleBar{Time: period, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume},
		minuteVols:  map[int64]int64{minute: c.Volume},
		closeMinute: minute,
	}
}

func seededPeriodState(seed dto.CandleBar, seedMinute, seedMinuteVolume int64) *periodState {
	return &periodState{bar: seed, minuteVols: make(map[int64]int64), skipUnseenUpTo: seedMinute, seedMinuteVolume: seedMinuteVolume}
}

func (p *periodState) apply(c domain.Candle) {
	minute := c.Timestamp.Unix()
	if c.High > p.bar.High {
		p.bar.High = c.High
	}
	if c.Low < p.bar.Low {
		p.bar.Low = c.Low
	}
	if minute >= p.closeMinute {
		p.bar.Close = c.Close
		p.closeMinute = minute
	}
	previous, seen := p.minuteVols[minute]
	switch {
	case seen:
		p.bar.Volume += c.Volume - previous
	case p.seedMinuteVolume > 0 && minute == p.skipUnseenUpTo:
		p.bar.Volume += max(c.Volume-p.seedMinuteVolume, 0)
	case p.skipUnseenUpTo != 0 && minute <= p.skipUnseenUpTo:
	default:
		p.bar.Volume += c.Volume
	}
	p.minuteVols[minute] = c.Volume
}
