package tastytrade

import "testing"

func TestTradingStatusOf(t *testing.T) {
	tr := true
	fl := false
	if got := tradingStatusOf(nil); got != "ACTIVE" {
		t.Fatalf("tradingStatusOf(nil) = %q, want ACTIVE", got)
	}
	if got := tradingStatusOf(&tr); got != "HALTED" {
		t.Fatalf("tradingStatusOf(true) = %q, want HALTED", got)
	}
	if got := tradingStatusOf(&fl); got != "ACTIVE" {
		t.Fatalf("tradingStatusOf(false) = %q, want ACTIVE", got)
	}
}
