package tastytrade

import (
	"sync"
	"time"
)

func lockedString(mu *sync.Mutex, v *string) string {
	mu.Lock()
	defer mu.Unlock()
	return *v
}

func copyCounts(src map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func unixTimeFromNano(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

func ageSinceUnixNano(n int64) time.Duration {
	if n == 0 {
		return 0
	}
	return time.Since(time.Unix(0, n))
}
