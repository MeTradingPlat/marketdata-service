package catchup

import (
	"testing"
	"time"
)

func TestNextMaintenanceWindowAt(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("loading America/New_York: %v", err)
	}

	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before close today, EDT (summer)",
			now:  time.Date(2026, 7, 15, 12, 0, 0, 0, loc),
			want: time.Date(2026, 7, 15, 20, 5, 0, 0, loc),
		},
		{
			name: "after close today, EDT (summer) rolls to tomorrow",
			now:  time.Date(2026, 7, 15, 21, 0, 0, 0, loc),
			want: time.Date(2026, 7, 16, 20, 5, 0, 0, loc),
		},
		{
			name: "before close today, EST (winter)",
			now:  time.Date(2026, 1, 15, 12, 0, 0, 0, loc),
			want: time.Date(2026, 1, 15, 20, 5, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextMaintenanceWindowAt(tt.now)
			if !got.Equal(tt.want) {
				t.Errorf("NextMaintenanceWindowAt(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

// TestNextMaintenanceWindowAt_DSTOffsetDiffers confirma el bug que motivo el
// cambio: con UTC fijo, 8:05pm ET cae en horas UTC distintas segun la epoca
// del anio (EDT/EST), asi que un offset fijo en UTC no puede representar
// "5 minutos despues del cierre" en las dos estaciones a la vez.
func TestNextMaintenanceWindowAt_DSTOffsetDiffers(t *testing.T) {
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	summerTarget := NextMaintenanceWindowAt(summer)
	winterTarget := NextMaintenanceWindowAt(winter)

	if summerTarget.UTC().Hour() == winterTarget.UTC().Hour() {
		t.Errorf("expected the UTC hour of the maintenance window to differ between EDT and EST, got %d for both", summerTarget.UTC().Hour())
	}
}
