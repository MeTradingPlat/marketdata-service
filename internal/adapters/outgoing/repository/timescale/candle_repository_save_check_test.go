package timescale

import "testing"

func TestCheckBatchRowsAffected(t *testing.T) {
	before := CandleUpsertShortBatches()

	if checkBatchRowsAffected(10, 10, true) {
		t.Fatal("expected false when affected equals expected")
	}
	if checkBatchRowsAffected(10, 0, false) {
		t.Fatal("expected false when withWatermark is false due to verified guard")
	}
	if CandleUpsertShortBatches() != before {
		t.Fatalf("counter changed unexpectedly: got %d, want %d", CandleUpsertShortBatches(), before)
	}

	if !checkBatchRowsAffected(10, 7, true) {
		t.Fatal("expected true when withWatermark is true and affected < expected")
	}
	if got := CandleUpsertShortBatches(); got != before+1 {
		t.Fatalf("counter = %d, want %d", got, before+1)
	}
}
