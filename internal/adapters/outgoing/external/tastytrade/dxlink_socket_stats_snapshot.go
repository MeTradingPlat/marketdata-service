package tastytrade

func (s *dxLinkSocketStats) snapshot() map[string]any {
	s.mu.Lock()
	errs := copyCounts(s.serverErrors)
	unk := copyCounts(s.unknownTypes)
	abandoned := int64(len(s.abandonedChannels))
	s.mu.Unlock()
	return map[string]any{
		"opens_requested": s.opensRequested.Load(),
		"opens_succeeded": s.opensSucceeded.Load(),
		"opens_timed_out": s.opensTimedOut.Load(),
		"opens_canceled":  s.opensCanceled.Load(),
		"cancels_sent":    s.cancelsSent.Load(),
		"late_responses":  s.lateResponses.Load(),
		"channels_closed": s.channelsClosed.Load(),
		"abandoned_count": abandoned,
		"server_errors":   errs,
		"unknown_types":   unk,
	}
}
