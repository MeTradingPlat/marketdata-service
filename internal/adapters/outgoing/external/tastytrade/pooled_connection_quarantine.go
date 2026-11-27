package tastytrade

import "time"

const channelOpenTimeoutThreshold = 3

func quarantineDuration(strikes int) time.Duration {
	if strikes <= 0 {
		return 0
	}
	d := 2 * time.Minute << uint(min(strikes-1, 4))
	if d > 30*time.Minute {
		return 30 * time.Minute
	}
	return d
}

func (p *pooledConnection) isQuarantined(now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return now.Before(p.quarantinedUntil)
}

func (p *pooledConnection) recordOpenSuccess() {
	p.mu.Lock()
	p.consecutiveOpenTimeouts = 0
	p.quarantineStrikes = 0
	p.quarantinedUntil = time.Time{}
	p.mu.Unlock()
}

func (p *pooledConnection) recordOpenTimeout(now time.Time) (bool, int, time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveOpenTimeouts++
	if p.consecutiveOpenTimeouts < channelOpenTimeoutThreshold {
		return false, 0, 0
	}
	p.consecutiveOpenTimeouts = 0
	p.quarantineStrikes++
	dur := quarantineDuration(p.quarantineStrikes)
	p.quarantinedUntil = now.Add(dur)
	return true, p.quarantineStrikes, dur
}

func (p *pooledConnection) quarantineRank() (int, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.quarantineStrikes, p.quarantinedUntil
}

func lessQuarantineRank(s1 int, u1 time.Time, s2 int, u2 time.Time) bool {
	if s1 != s2 {
		return s1 < s2
	}
	return u1.Before(u2)
}
