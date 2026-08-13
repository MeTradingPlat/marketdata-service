package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// La sesion de TastyTrade se degrada pasados ~15min sin refrescar -- 14min
// de margen evita llegar a ese borde bajo carga.
const proactiveRefreshInterval = 14 * time.Minute

func StartProactiveRefresh(ctx context.Context, oauth *OAuth, quoteToken *QuoteToken) {
	ticker := time.NewTicker(proactiveRefreshInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := oauth.RefreshAccessToken(ctx); err != nil {
					log.Error().Err(err).Msg("proactive access token refresh failed")
					continue
				}
				if err := quoteToken.Refresh(ctx); err != nil {
					log.Error().Err(err).Msg("proactive api quote token refresh failed")
				}
			}
		}
	}()
}
