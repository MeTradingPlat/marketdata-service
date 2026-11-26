package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func (p *CandlePool) canHealSilent(now time.Time, limit int) bool {
	if limit <= 0 || !p.Connected() || !isRegularSessionET(now) {
		return false
	}
	age := p.LastLiveEventAge()
	return age > 0 && age <= liveDataSilenceThreshold
}

func (p *CandlePool) HealSilentSymbols(ctx context.Context, now time.Time, limit int) int {
	if !p.canHealSilent(now, limit) {
		return 0
	}
	healed := 0
	consecutiveFailures := 0
	for _, sym := range p.silentHealCandidates(now, limit) {
		if consecutiveFailures >= silentHealMaxConsecutiveFailures {
			break
		}
		ok, err := p.repairSilentSymbol(ctx, sym, now)
		if err != nil {
			consecutiveFailures++
			log.Warn().Err(err).Str("symbol", sym).Msg("failed to heal silent live symbol")
			continue
		}
		if ok {
			healed++
			consecutiveFailures = 0
		}
	}
	return healed
}

func (p *CandlePool) repairSilentSymbol(ctx context.Context, symbol string, now time.Time) (bool, error) {
	oldCh := p.liveChannelFor(symbol)
	if oldCh == nil {
		return false, nil
	}
	if !p.hasLiveDispatch(symbol) {
		p.registerDispatch(symbol, domain.M1, "live", func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })
		p.recordSilentRepair(symbol, now)
		log.Warn().Str("symbol", symbol).Str("mode", "dispatch").Int("old_channel", oldCh.channel.id).Int("new_channel", oldCh.channel.id).Msg("healed silent live symbol")
		return true, nil
	}
	return p.moveSilentSymbolChannel(ctx, symbol, oldCh, now)
}
