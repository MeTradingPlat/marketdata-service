package tastytrade

import "time"

const (
	silentHealSilenceAfter           = 5 * time.Minute
	silentHealTradeWindow            = 5 * time.Minute
	silentHealMaxConsecutiveFailures = 3
	silentHealMaxPerHour             = 3
	silentHealRateWindow             = time.Hour
	silentHealFallbackLookback       = 10 * time.Minute
)

func (p *CandlePool) isStaleWithActivity(symbol string, now time.Time) bool {
	p.activityMu.Lock()
	lastBar := p.lastNewBarAt[symbol]
	subAt := p.subscribedAt[symbol]
	lastTrade := p.lastTradeAt[symbol]
	p.activityMu.Unlock()
	if !lastBar.IsZero() {
		if now.Sub(lastBar) <= silentHealSilenceAfter {
			return false
		}
	} else if !subAt.IsZero() && now.Sub(subAt) <= silentHealSilenceAfter {
		return false
	}
	return !lastTrade.IsZero() && now.Sub(lastTrade) <= silentHealTradeWindow
}
