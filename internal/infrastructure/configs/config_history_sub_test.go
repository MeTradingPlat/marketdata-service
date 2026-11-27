package configs

import "testing"

func TestLoad_HistorySubChangesPerMinute(t *testing.T) {
	t.Setenv("HISTORY_SUB_CHANGES_PER_MINUTE", "")
	if got := Load().HistorySubChangesPerMinute; got != 8000 {
		t.Fatalf("default = %d, want 8000", got)
	}
	t.Setenv("HISTORY_SUB_CHANGES_PER_MINUTE", "5000")
	if got := Load().HistorySubChangesPerMinute; got != 5000 {
		t.Fatalf("5000 = %d, want 5000", got)
	}
	t.Setenv("HISTORY_SUB_CHANGES_PER_MINUTE", "0")
	if got := Load().HistorySubChangesPerMinute; got != 8000 {
		t.Fatalf("0 = %d, want 8000", got)
	}
	t.Setenv("HISTORY_SUB_CHANGES_PER_MINUTE", "-5")
	if got := Load().HistorySubChangesPerMinute; got != 8000 {
		t.Fatalf("-5 = %d, want 8000", got)
	}
}
