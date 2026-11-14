package main

import (
	"testing"
	"time"
)

func TestNextResetTime(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

	got := nextResetTime(now, 22, 15)
	want := time.Date(2026, 11, 14, 22, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("before hour: got %v, want %v", got, want)
	}

	got = nextResetTime(now, 9, 0)
	want = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("passed hour: got %v, want %v", got, want)
	}
}
