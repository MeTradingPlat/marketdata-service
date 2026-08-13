package domain

import (
	"fmt"
	"time"
)

type Timeframe string

const (
	M1 Timeframe = "M1"
	H1 Timeframe = "H1"
	D1 Timeframe = "D1"
)

var BaseTimeframes = []Timeframe{M1, H1, D1}

func (t Timeframe) Duration() (time.Duration, error) {
	switch t {
	case M1:
		return time.Minute, nil
	case H1:
		return time.Hour, nil
	case D1:
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown timeframe: %s", t)
	}
}

func (t Timeframe) Valid() bool {
	switch t {
	case M1, H1, D1:
		return true
	default:
		return false
	}
}
