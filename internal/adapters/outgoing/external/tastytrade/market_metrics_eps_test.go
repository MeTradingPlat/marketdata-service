package tastytrade

import "testing"

func TestParseFloatPtr_DistinguishesZeroFromMissing(t *testing.T) {
	if got := parseFloatPtr(nil); got != nil {
		t.Fatalf("parseFloatPtr(nil) = %v, want nil", *got)
	}
	empty := ""
	if got := parseFloatPtr(&empty); got != nil {
		t.Fatalf("parseFloatPtr(empty) = %v, want nil", *got)
	}
	zero := "0.00"
	if got := parseFloatPtr(&zero); got == nil || *got != 0 {
		t.Fatalf("parseFloatPtr(0.00) = %v, want 0", got)
	}
	pos := "1.25"
	if got := parseFloatPtr(&pos); got == nil || *got != 1.25 {
		t.Fatalf("parseFloatPtr(1.25) = %v, want 1.25", got)
	}
}
