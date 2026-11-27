package main

import (
	"context"
	"testing"
	"time"
)

type fakeLiveHealer struct{}

func (fakeLiveHealer) HealMuteChannels(context.Context, time.Time, int) int  { return 0 }
func (fakeLiveHealer) ReattachOrphans(context.Context, time.Time, int) int   { return 0 }
func (fakeLiveHealer) HealSilentSymbols(context.Context, time.Time, int) int { return 0 }
func (fakeLiveHealer) LogLiveHealth(time.Time)                               {}

func TestStartLiveHealLoop_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	StartLiveHealLoop(ctx, fakeLiveHealer{})
}

type deadlineRecordingHealer struct {
	deadlines []time.Time
}

func (h *deadlineRecordingHealer) record(ctx context.Context) {
	if d, ok := ctx.Deadline(); ok {
		h.deadlines = append(h.deadlines, d)
	}
}

func (h *deadlineRecordingHealer) HealMuteChannels(ctx context.Context, _ time.Time, _ int) int {
	h.record(ctx)
	return 0
}
func (h *deadlineRecordingHealer) ReattachOrphans(ctx context.Context, _ time.Time, _ int) int {
	h.record(ctx)
	return 0
}
func (h *deadlineRecordingHealer) HealSilentSymbols(ctx context.Context, _ time.Time, _ int) int {
	h.record(ctx)
	return 0
}
func (h *deadlineRecordingHealer) LogLiveHealth(time.Time) {}

func TestRunLiveHealTick_EachCallHas3MinDeadline(t *testing.T) {
	h := &deadlineRecordingHealer{}
	now := time.Now()
	var lastChannelHeal time.Time
	runLiveHealTick(context.Background(), h, now, &lastChannelHeal)
	if len(h.deadlines) != 3 {
		t.Fatalf("expected 3 deadlines, got %d", len(h.deadlines))
	}
	for i, d := range h.deadlines {
		rem := time.Until(d)
		if rem < 2*time.Minute || rem > 3*time.Minute+time.Second {
			t.Fatalf("call %d deadline out of 3m range: %v", i, rem)
		}
	}
}
