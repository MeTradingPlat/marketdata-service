package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

type accessTokenRefresher interface {
	RefreshAccessToken(ctx context.Context) (string, error)
}

type quoteTokenRefresher interface {
	Refresh(ctx context.Context) error
}

func refreshTokens(ctx context.Context, access accessTokenRefresher, quote quoteTokenRefresher, delays []time.Duration) {
	err := retryWithBackoff(ctx, delays, func() error {
		_, err := access.RefreshAccessToken(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("proactive access token refresh attempt failed")
		}
		return err
	})
	if err != nil {
		log.Error().Err(err).Msg("proactive access token refresh failed")
		return
	}
	if err := retryWithBackoff(ctx, delays, func() error { return quote.Refresh(ctx) }); err != nil {
		log.Error().Err(err).Msg("proactive api quote token refresh failed")
	}
}
