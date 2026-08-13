package domain

import "time"

type Quote struct {
	Symbol    string
	Bid       float64
	Ask       float64
	BidSize   int64
	AskSize   int64
	Timestamp time.Time
}
