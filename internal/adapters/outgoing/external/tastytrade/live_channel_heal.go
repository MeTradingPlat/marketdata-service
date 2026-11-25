package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

const (
	muteChannelMinMembers = 50
	muteChannelMuteRatio  = 0.9
	muteChannelAfter      = 15 * time.Minute
	muteChannelMinAge     = 30 * time.Minute
)

type channelMuteStats struct {
	Members      int
	MuteMembers  int
	LastActivity time.Time
}

func (s channelMuteStats) isMute(now time.Time) bool {
	if s.Members < muteChannelMinMembers || s.LastActivity.IsZero() || now.Sub(s.LastActivity) < muteChannelAfter {
		return false
	}
	return float64(s.MuteMembers) >= float64(s.Members)*muteChannelMuteRatio
}

func (p *CandlePool) channelMuteStats(ch *pooledChannel, now time.Time) channelMuteStats {
	symbols := ch.liveSymbols()
	stats := channelMuteStats{Members: len(symbols)}
	p.activityMu.Lock()
	for _, sym := range symbols {
		last, ok := p.lastEventAt[sym]
		if !ok || last.IsZero() || now.Sub(last) > muteChannelAfter {
			stats.MuteMembers++
		}
		if !last.IsZero() && last.After(stats.LastActivity) {
			stats.LastActivity = last
		}
	}
	p.activityMu.Unlock()
	return stats
}

// HealMuteChannels recicla canales dxLink mudos solo en sesion regular (en
// pre y post market el silencio de un canal de 100 simbolos es normal) dejando
// sus simbolos como huerfanos para que ReattachOrphans los reenganche.
func (p *CandlePool) HealMuteChannels(ctx context.Context, now time.Time, limit int) int {
	if limit <= 0 || !p.Connected() || !isRegularSessionET(now) {
		return 0
	}
	age := p.LastLiveEventAge()
	if age == 0 || age > liveDataSilenceThreshold {
		return 0
	}
	recycled := 0
	for _, ch := range p.allocator.allChannels() {
		if now.Sub(ch.createdAt) < muteChannelMinAge {
			continue
		}
		stats := p.channelMuteStats(ch, now)
		if !stats.isMute(now) {
			continue
		}
		if p.recycleChannel(ch) {
			recycled++
			log.Warn().Int("channel", ch.channel.id).Int("members", stats.Members).
				Int("mute", stats.MuteMembers).Time("last_activity", stats.LastActivity).
				Msg("recycled mute dxlink channel, symbols left for orphan reattach")
			if recycled >= limit {
				break
			}
		}
	}
	return recycled
}

func (p *CandlePool) recycleChannel(ch *pooledChannel) bool {
	for _, sym := range ch.liveSymbols() {
		ch.release(candleKey(sym, domain.M1))
	}
	return p.allocator.removeChannel(ch)
}
