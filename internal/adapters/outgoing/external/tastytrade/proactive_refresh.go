package tastytrade

import (
	"context"
	"time"
)

// La sesion de TastyTrade se degrada pasados ~15min sin refrescar -- renovar
// cada 10min deja ~5min de margen para los reintentos si una renovacion falla
// por timeout (el 2026-11-17 una sola falla sin reintento dejo el token
// vencido ~14min y 133/133 consultas by-type dieron 401).
const proactiveRefreshInterval = 10 * time.Minute

var proactiveRetryDelays = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

func StartProactiveRefresh(ctx context.Context, oauth *OAuth, quoteToken *QuoteToken) {
	ticker := time.NewTicker(proactiveRefreshInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshTokens(ctx, oauth, quoteToken, proactiveRetryDelays)
			}
		}
	}()
}
