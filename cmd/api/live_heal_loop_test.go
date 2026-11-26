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
