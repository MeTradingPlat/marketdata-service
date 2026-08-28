package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

const recentCacheEvictInterval = 5 * time.Minute

// StartRecentCacheEvictLoop descarta del RecentCache lo mas viejo que su
// ventana de retencion (DefaultRecentCacheTTL) -- sin esto, cada simbolo
// que alguna vez tuvo un tick en vivo se queda para siempre en memoria.
func StartRecentCacheEvictLoop(ctx context.Context, cache *livecandles.RecentCache) {
	go func() {
		ticker := time.NewTicker(recentCacheEvictInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cache.Evict(time.Now())
			}
		}
	}()
}
