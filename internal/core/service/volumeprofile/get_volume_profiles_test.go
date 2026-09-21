package volumeprofile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type fakeRepo struct {
	profiles map[string]domain.VolumeProfile
}

func (f fakeRepo) GetSymbolsWithStaleVolumeProfile(context.Context, time.Time) ([]string, error) {
	return nil, nil
}

func (f fakeRepo) ComputeProfiles(context.Context, []string, time.Time) (map[string]domain.VolumeProfile, error) {
	return nil, nil
}

func (f fakeRepo) SaveBatch(context.Context, map[string]domain.VolumeProfile, []string) error {
	return nil
}

func (f fakeRepo) GetBatch(context.Context, []string) (map[string]domain.VolumeProfile, error) {
	return f.profiles, nil
}

func TestGetVolumeProfiles_ReduceElPerfilAlTimeframePedido(t *testing.T) {
	cumulative := make([]float32, domain.VolumeProfileSlots)
	for i := range cumulative {
		cumulative[i] = float32(i + 1)
	}
	svc := NewGetVolumeProfilesService(fakeRepo{profiles: map[string]domain.VolumeProfile{
		"AAPL": {Symbol: "AAPL", Sessions: 20, Cumulative: cumulative},
	}})

	got, err := svc.GetVolumeProfiles(context.Background(), []string{"AAPL", "SIN_PERFIL"}, domain.M5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	aapl, ok := got["AAPL"]
	if !ok || aapl.SlotMinutes != 5 || aapl.Sessions != 20 || len(aapl.Cumulative) != 192 || aapl.Cumulative[0] != 5 {
		t.Fatalf("perfil inesperado: %+v", aapl)
	}
	if _, exists := got["SIN_PERFIL"]; exists {
		t.Fatal("un simbolo sin perfil no debe aparecer")
	}
}

func TestGetVolumeProfiles_RechazaTimeframesSinPerfil(t *testing.T) {
	svc := NewGetVolumeProfilesService(fakeRepo{})

	_, err := svc.GetVolumeProfiles(context.Background(), []string{"AAPL"}, domain.D1)

	if !errors.Is(err, ErrUnsupportedTimeframe) {
		t.Fatalf("err = %v, want ErrUnsupportedTimeframe", err)
	}
}
