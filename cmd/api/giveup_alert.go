package main

import (
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/alerts"
)

const giveUpAlertThreshold = 20

var (
	giveUpNow     = time.Now
	notifyGiveUps = alerts.Notify

	giveUpMu        sync.Mutex
	giveUpTimes     []time.Time
	giveUpLastAlert time.Time
)

func recordGiveUp(symbol string) {
	giveUpMu.Lock()
	defer giveUpMu.Unlock()

	now := giveUpNow()
	giveUpTimes = append(giveUpTimes, now)

	cutoff := now.Add(-time.Hour)
	start := 0
	for start < len(giveUpTimes) && giveUpTimes[start].Before(cutoff) {
		start++
	}
	giveUpTimes = giveUpTimes[start:]

	if len(giveUpTimes) >= giveUpAlertThreshold && (giveUpLastAlert.IsZero() || now.Sub(giveUpLastAlert) > time.Hour) {
		notifyGiveUps("ERROR", "Simbolos sin stream M1 en vivo tras agotar reintentos", map[string]any{
			"abandonos1h":   len(giveUpTimes),
			"ultimoSimbolo": symbol,
		})
		giveUpLastAlert = now
	}
}
