package tastytrade

import "time"

const reviveMaxPerTick = 50

type reviveState struct {
	reconnectedAt time.Time
	lastAttemptAt time.Time
	attempts      int
}

func shouldRevive(st reviveState, lastEvent, now time.Time) bool {
	if now.Sub(st.reconnectedAt) < 3*time.Minute {
		return false
	}
	if !lastEvent.Before(st.reconnectedAt.Add(reviveSustainedAfter)) {
		return false
	}
	if st.attempts >= 2 {
		return false
	}
	if !st.lastAttemptAt.IsZero() && now.Sub(st.lastAttemptAt) < 3*time.Minute {
		return false
	}
	return true
}
