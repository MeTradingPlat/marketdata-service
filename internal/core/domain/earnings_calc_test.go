package domain

import "testing"

func TestLastEarningsReport(t *testing.T) {
	tests := []struct {
		name         string
		reports      []EarningsReportItem
		wantOccurred string
		wantNext     string
	}{
		{
			name:         "sin reportes",
			reports:      nil,
			wantOccurred: "",
			wantNext:     "",
		},
		{
			name: "fecha invalida se ignora",
			reports: []EarningsReportItem{
				{OccurredDate: "no-es-fecha"},
			},
			wantOccurred: "",
			wantNext:     "",
		},
		{
			name: "ciclo trimestral regular",
			reports: []EarningsReportItem{
				{OccurredDate: "2025-01-30"},
				{OccurredDate: "2025-04-30"},
				{OccurredDate: "2025-07-31"},
				{OccurredDate: "2025-10-30"},
			},
			wantOccurred: "2025-10-30",
			wantNext:     "2026-01-29",
		},
		{
			name: "delta corto se descarta (ruido, no ciclo)",
			reports: []EarningsReportItem{
				{OccurredDate: "2025-01-30"},
				{OccurredDate: "2025-02-05"},
				{OccurredDate: "2025-04-30"},
			},
			wantOccurred: "2025-04-30",
			wantNext:     "2025-07-23",
		},
		{
			name: "desordenado en la fuente",
			reports: []EarningsReportItem{
				{OccurredDate: "2025-10-30"},
				{OccurredDate: "2025-04-30"},
				{OccurredDate: "2025-07-31"},
				{OccurredDate: "2025-01-30"},
			},
			wantOccurred: "2025-10-30",
			wantNext:     "2026-01-29",
		},
		{
			name: "mediana con cantidad par de deltas",
			reports: []EarningsReportItem{
				{OccurredDate: "2025-01-30"},
				{OccurredDate: "2025-04-30"},
				{OccurredDate: "2025-07-31"},
			},
			wantOccurred: "2025-07-31",
			wantNext:     "2025-10-30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occurred, next := LastEarningsReport(tt.reports)
			if occurred != tt.wantOccurred {
				t.Errorf("occurred = %q, want %q", occurred, tt.wantOccurred)
			}
			if next != tt.wantNext {
				t.Errorf("next = %q, want %q", next, tt.wantNext)
			}
		})
	}
}
