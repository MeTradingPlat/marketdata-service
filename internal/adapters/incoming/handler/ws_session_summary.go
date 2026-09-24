package handler

import (
	"time"

	"github.com/rs/zerolog/log"
)

func (s *baseWSSession) logClosed(subscriptions int) {
	log.Info().
		Str("client", s.client).
		Str("session", s.errContext).
		Int("subscriptions", subscriptions).
		Int64("writeFailures", s.writeFailures.Load()).
		Dur("connectedFor", time.Since(s.startedAt).Round(time.Second)).
		Msg("ws session closed")
}
