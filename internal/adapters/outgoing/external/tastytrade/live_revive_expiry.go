package tastytrade

import "time"

const (
	reviveSustainedAfter = 2 * time.Minute
	reviveExpireAfter    = 30 * time.Minute
)

func hasSustainedFlow(st reviveState, lastEvent time.Time) bool {
	return lastEvent.After(st.reconnectedAt.Add(reviveSustainedAfter))
}

func shouldDropRevive(st reviveState, lastEvent, now time.Time) bool {
	return hasSustainedFlow(st, lastEvent) || now.Sub(st.reconnectedAt) >= reviveExpireAfter
}
