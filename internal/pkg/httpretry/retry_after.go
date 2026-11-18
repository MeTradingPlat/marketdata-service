package httpretry

import (
	"net/http"
	"strconv"
	"time"
)

const maxRetryAfter = time.Minute

func parseRetryAfter(header string, now time.Time) time.Duration {
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return capRetryAfter(time.Duration(seconds) * time.Second)
	}
	if at, err := http.ParseTime(header); err == nil && at.After(now) {
		return capRetryAfter(at.Sub(now))
	}
	return 0
}

func capRetryAfter(wait time.Duration) time.Duration {
	return min(wait, maxRetryAfter)
}
