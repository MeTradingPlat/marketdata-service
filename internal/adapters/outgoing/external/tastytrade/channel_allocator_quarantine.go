package tastytrade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/alerts"
	"github.com/rs/zerolog/log"
)

const forcedReconnectCooldown = time.Minute

var notifyQuarantineAlert = alerts.Notify

func (a *channelAllocator) recordOpenResult(ctx context.Context, pc *pooledConnection, err error) {
	if err == nil {
		pc.recordOpenSuccess()
		return
	}
	if !errors.Is(err, errChannelOpenTimeout) {
		return
	}
	now := time.Now()
	quarantined, strikes, dur := pc.recordOpenTimeout(now)
	if !quarantined {
		return
	}
	forceReconnect := a.canForceReconnect(now)
	log.Warn().Int64("connID", pc.conn.id).Int("strikes", strikes).Dur("duration", dur).Int("open_channels", pc.openChannelCount()).Msg("dxlink connection quarantined after channel open timeouts")
	notifyQuarantineAlert("WARN", "dxlink connection quarantined after channel open timeouts", map[string]any{
		"connID":             pc.conn.id,
		"strikes":            strikes,
		"duration":           dur.String(),
		"force_reconnect":    forceReconnect,
		"open_channels":      pc.openChannelCount(),
		"last_message_age":   pc.conn.lastMessageAge().String(),
		"open_sessions":      openDxLinkSessions.Load(),
		"last_timeout_cause": pc.conn.stats.lastTimeoutCause(),
		"socket_stats":       pc.conn.stats.snapshot(),
	})
	if !forceReconnect {
		return
	}
	pc.conn.noteCloseReason(fmt.Sprintf("quarantine_forced_reconnect: strikes=%d duration=%s", strikes, dur))
	pc.conn.sendCloseFrame()
	pc.conn.ForceReconnect(context.WithoutCancel(ctx))
}

func (a *channelAllocator) canForceReconnect(now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.lastForcedReconnectAt.IsZero() && now.Sub(a.lastForcedReconnectAt) < forcedReconnectCooldown {
		return false
	}
	a.lastForcedReconnectAt = now
	return true
}
