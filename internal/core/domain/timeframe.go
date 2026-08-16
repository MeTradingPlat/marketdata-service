package domain

import (
	"fmt"
	"time"
)

type Timeframe string

const (
	M1  Timeframe = "M1"
	M2  Timeframe = "M2"
	M3  Timeframe = "M3"
	M5  Timeframe = "M5"
	M10 Timeframe = "M10"
	M15 Timeframe = "M15"
	M30 Timeframe = "M30"
	M45 Timeframe = "M45"
	H1  Timeframe = "H1"
	H2  Timeframe = "H2"
	H3  Timeframe = "H3"
	H4  Timeframe = "H4"
	H12 Timeframe = "H12"
	D1  Timeframe = "D1"
	D2  Timeframe = "D2"
	D3  Timeframe = "D3"
	W1  Timeframe = "W1"
	MO1 Timeframe = "MO1"
	MO3 Timeframe = "MO3"
	MO6 Timeframe = "MO6"
	Y1  Timeframe = "Y1"
)

// BaseTimeframes son los unicos que TastyTrade entrega nativos -- el resto
// se deriva agrupando estos (ver timeframe_aggregation.go).
var BaseTimeframes = []Timeframe{M1, H1, D1}

func (t Timeframe) IsBase() bool {
	switch t {
	case M1, H1, D1:
		return true
	default:
		return false
	}
}

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
	if t.IsBase() {
		return true
	}
	_, ok := derivedTimeframes[t]
	return ok
}
