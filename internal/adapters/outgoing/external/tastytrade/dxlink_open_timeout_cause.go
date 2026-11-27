package tastytrade

import "time"

func classifyOpenTimeout(lastMsgAt, lastErrAt, sentAt time.Time, openedReceived bool, lateResponses int64) string {
	if lastMsgAt.Before(sentAt) {
		return "network_silent"
	}
	if !lastErrAt.IsZero() && !lastErrAt.Before(sentAt) {
		return "server_error"
	}
	if openedReceived {
		return "partial_response"
	}
	if lateResponses > 0 {
		return "server_slow"
	}
	return "requests_ignored"
}
