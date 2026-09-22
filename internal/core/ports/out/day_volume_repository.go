package out

import (
	"context"
	"time"
)

// DayVolumeRepository persiste el volumen real del dia (Trade.dayVolume de
// DxLink) por simbolo -- sobrevive un reinicio de marketdata-service, asi
// DayVolumeTracker puede sembrarse desde aca mientras espera el primer
// refresco en vivo (que puede tardar hasta 5 min).
type DayVolumeRepository interface {
	SaveBatch(ctx context.Context, day time.Time, volumes map[string]int64) error
	GetBatch(ctx context.Context, symbols []string, day time.Time) (map[string]int64, error)
}
