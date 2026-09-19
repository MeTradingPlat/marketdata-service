package handler

const (
	defaultHistoryBars = 500
	maxHistoryBars     = 2000
)

func historyBars(requested int) int {
	if requested <= 0 {
		return defaultHistoryBars
	}
	return min(requested, maxHistoryBars)
}
