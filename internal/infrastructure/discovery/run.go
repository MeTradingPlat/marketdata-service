package discovery

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	heartbeatInterval     = 30 * time.Second
	registerRetryInterval = 5 * time.Second
)

// Run registra la instancia en Eureka y la mantiene viva con heartbeats
// hasta que ctx se cancele. Reintenta el registro inicial indefinidamente
// (directory puede no estar listo todavia cuando este contenedor arranca,
// el CD no espera healthcheck entre contenedores) y se re-registra si un
// heartbeat falla -- Eureka ya desalojo ese instanceId y seguir pegandole
// heartbeats no lo revive.
func Run(ctx context.Context, client *Client) {
	for {
		if err := client.Register(ctx); err != nil {
			log.Error().Err(err).Msg("eureka registration failed, retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(registerRetryInterval):
				continue
			}
		}
		log.Info().Msg("registered with eureka")
		break
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			deregisterCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.Deregister(deregisterCtx); err != nil {
				log.Error().Err(err).Msg("eureka deregistration failed")
			}
			return
		case <-ticker.C:
			if err := client.Heartbeat(ctx); err != nil {
				log.Error().Err(err).Msg("eureka heartbeat failed, re-registering")
				if err := client.Register(ctx); err != nil {
					log.Error().Err(err).Msg("eureka re-registration failed")
				}
			}
		}
	}
}
