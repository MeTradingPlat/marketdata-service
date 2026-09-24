package main

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

func TestDueDayBoundary(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	at := func(day, hour, minute, second int) time.Time {
		return time.Date(2026, 9, day, hour, minute, second, 0, loc)
	}
	cases := map[string]struct {
		when     time.Time
		wantKind intraday.DayBoundary
		wantDue  bool
	}{
		"too early for pre-market end": {at(23, 9, 27, 0), 0, false},
		"start of pre-market end":      {at(23, 9, 27, 30), intraday.PreMarketEnd, true},
		"end of pre-market window":     {at(23, 9, 28, 59), intraday.PreMarketEnd, true},
		"too late for pre-market end":  {at(23, 9, 29, 0), 0, false},
		"before the closing cross":     {at(23, 16, 0, 30), 0, false},
		"start of regular end":         {at(23, 16, 1, 0), intraday.RegularEnd, true},
		"end of regular window":        {at(23, 16, 3, 59), intraday.RegularEnd, true},
		"after the regular window":     {at(23, 16, 4, 0), 0, false},
		"weekend":                      {at(26, 9, 29, 0), 0, false},
	}
	for name, tc := range cases {
		kind, due := dueDayBoundary(tc.when)
		if due != tc.wantDue || (due && kind != tc.wantKind) {
			t.Errorf("%s: got kind=%v due=%v, want kind=%v due=%v", name, kind, due, tc.wantKind, tc.wantDue)
		}
	}
}

func TestIsBoundaryCaptureTooLate(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	at := func(hour, minute, second int) time.Time { return time.Date(2026, 9, 23, hour, minute, second, 0, loc) }

	if isBoundaryCaptureTooLate(intraday.PreMarketEnd, at(9, 29, 59)) {
		t.Error("a pre-market capture that ended before the open is still valid")
	}
	if !isBoundaryCaptureTooLate(intraday.PreMarketEnd, at(9, 39, 7)) {
		t.Error("a pre-market capture that ended after the open includes regular-session volume and must be discarded")
	}
	if isBoundaryCaptureTooLate(intraday.RegularEnd, at(16, 9, 0)) {
		t.Error("the regular-close boundary has no upper deadline")
	}
}
