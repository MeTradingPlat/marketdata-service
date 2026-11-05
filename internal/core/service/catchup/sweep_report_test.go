package catchup

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestReportPhase(t *testing.T) {
	orig := Alert
	var recordedLevel string

	Alert = func(level, message string, metadata map[string]any) {
		recordedLevel = level
	}
	t.Cleanup(func() {
		Alert = orig
	})

	reportPhase(domain.D1, 10, 0, 1, 5*time.Second)
	if recordedLevel != "WARN" {
		t.Errorf("got level %s, want WARN", recordedLevel)
	}

	reportPhase(domain.D1, 0, 0, 0, 0)
	if recordedLevel != "INFO" {
		t.Errorf("got level %s, want INFO", recordedLevel)
	}
}
