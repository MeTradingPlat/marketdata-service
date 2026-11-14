package out

import "time"

type CandleForceCloser interface {
	CloseElapsedForming(now time.Time)
}
