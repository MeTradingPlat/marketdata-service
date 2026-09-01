package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestRawCandleEvent_SnapshotDone(t *testing.T) {
	tests := []struct {
		name  string
		flags int
		want  bool
	}{
		{"no flags", 0, false},
		{"snapshot end", eventFlagSnapshotEnd, true},
		{"snapshot snip", eventFlagSnapshotSnip, true},
		{"snapshot end but still tx pending", eventFlagSnapshotEnd | eventFlagTxPending, false},
		{"tx pending alone", eventFlagTxPending, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := rawCandleEvent{EventFlags: tt.flags}
			if got := ev.snapshotDone(); got != tt.want {
				t.Errorf("snapshotDone() with flags %d = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}

func completeCandleEvent(ts time.Time, flags int) rawCandleEvent {
	price := 1.0
	return rawCandleEvent{Timestamp: ts, Open: &price, High: &price, Low: &price, Close: &price, EventFlags: flags}
}

// TestHistoryCollector_SettledImmediatelyOnSnapshotEnd confirma el fix del
// 2026-08-31: si dxLink marca el final real de la rafaga, settled() no
// espera el periodo de silencio -- antes, un simbolo de mucho volumen podia
// seguir mandando datos activamente y el timeout de reloj cortaba a mitad
// de camino (ver el comentario de settled()).
func TestHistoryCollector_SettledImmediatelyOnSnapshotEnd(t *testing.T) {
	h := newHistoryCollector("TEST", domain.M1)
	h.onCandle(completeCandleEvent(time.Now(), eventFlagSnapshotEnd))

	if !h.settled() {
		t.Fatal("expected settled() to be true immediately after a SNAPSHOT_END event, without waiting historyQuietPeriod")
	}
}

func TestHistoryCollector_SettledFallsBackToQuietPeriodWithoutFlags(t *testing.T) {
	h := newHistoryCollector("TEST", domain.M1)
	h.onCandle(completeCandleEvent(time.Now(), 0))

	if h.settled() {
		t.Fatal("expected settled() to be false right after a candle with no eventFlags, before the quiet period elapses")
	}
	time.Sleep(historyQuietPeriod + 50*time.Millisecond)
	if !h.settled() {
		t.Fatal("expected settled() to be true once historyQuietPeriod elapsed with no further updates")
	}
}
