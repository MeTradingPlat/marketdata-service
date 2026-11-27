package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCandleJSON_OmitsObservedAt(t *testing.T) {
	ts := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	obs := time.Date(2026, 11, 27, 15, 1, 0, 0, time.UTC)
	c := Candle{
		Symbol:     "AAPL",
		Timeframe:  M1,
		Timestamp:  ts,
		Open:       100,
		High:       102,
		Low:        99,
		Close:      101,
		Volume:     500,
		TradeCount: 10,
		VWAP:       100.5,
		Source:     "tastytrade",
		ObservedAt: obs,
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)
	if strings.Contains(raw, "ObservedAt") || strings.Contains(raw, "observedAt") || strings.Contains(raw, "observed_at") {
		t.Fatalf("JSON must not include ObservedAt, got: %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(decoded) != 11 {
		t.Fatalf("got %d JSON fields, want 11: %s", len(decoded), raw)
	}
}
