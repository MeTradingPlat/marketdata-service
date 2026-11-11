package main

import (
	"fmt"
	"testing"
	"time"
)

func TestRecordGiveUpAlerts(t *testing.T) {
	giveUpMu.Lock()
	origNow := giveUpNow
	origNotify := notifyGiveUps
	giveUpTimes = nil
	giveUpLastAlert = time.Time{}
	giveUpMu.Unlock()

	var recordedAlerts []struct {
		level   string
		message string
		fields  map[string]any
	}

	fakeNow := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	giveUpNow = func() time.Time {
		return fakeNow
	}
	notifyGiveUps = func(level, message string, fields map[string]any) {
		recordedAlerts = append(recordedAlerts, struct {
			level   string
			message string
			fields  map[string]any
		}{level: level, message: message, fields: fields})
	}

	t.Cleanup(func() {
		giveUpMu.Lock()
		giveUpNow = origNow
		notifyGiveUps = origNotify
		giveUpTimes = nil
		giveUpLastAlert = time.Time{}
		giveUpMu.Unlock()
	})

	for i := 1; i <= 19; i++ {
		recordGiveUp(fmt.Sprintf("SYM%d", i))
	}
	if len(recordedAlerts) != 0 {
		t.Fatalf("expected 0 alerts for 19 give-ups, got %d", len(recordedAlerts))
	}

	recordGiveUp("SYM20")
	if len(recordedAlerts) != 1 {
		t.Fatalf("expected 1 alert on 20th give-up, got %d", len(recordedAlerts))
	}
	if recordedAlerts[0].level != "ERROR" {
		t.Errorf("expected level ERROR, got %s", recordedAlerts[0].level)
	}
	if recordedAlerts[0].fields["abandonos1h"] != 20 {
		t.Errorf("expected abandonos1h == 20, got %v", recordedAlerts[0].fields["abandonos1h"])
	}
	if recordedAlerts[0].fields["ultimoSimbolo"] != "SYM20" {
		t.Errorf("expected ultimoSimbolo == SYM20, got %v", recordedAlerts[0].fields["ultimoSimbolo"])
	}

	fakeNow = fakeNow.Add(15 * time.Minute)
	for i := 21; i <= 25; i++ {
		recordGiveUp(fmt.Sprintf("SYM%d", i))
	}
	if len(recordedAlerts) != 1 {
		t.Fatalf("expected still 1 alert within 1h, got %d", len(recordedAlerts))
	}

	fakeNow = fakeNow.Add(61 * time.Minute)
	for i := 1; i <= 19; i++ {
		recordGiveUp(fmt.Sprintf("NEW_SYM%d", i))
	}
	if len(recordedAlerts) != 1 {
		t.Fatalf("expected still 1 alert because entries older than 1h are not counted, got %d", len(recordedAlerts))
	}

	recordGiveUp("NEW_SYM20")
	if len(recordedAlerts) != 2 {
		t.Fatalf("expected 2nd alert after 61 min and 20 new give-ups, got %d", len(recordedAlerts))
	}
	if recordedAlerts[1].fields["abandonos1h"] != 20 {
		t.Errorf("expected abandonos1h == 20 on 2nd alert, got %v", recordedAlerts[1].fields["abandonos1h"])
	}
	if recordedAlerts[1].fields["ultimoSimbolo"] != "NEW_SYM20" {
		t.Errorf("expected ultimoSimbolo == NEW_SYM20, got %v", recordedAlerts[1].fields["ultimoSimbolo"])
	}
}
