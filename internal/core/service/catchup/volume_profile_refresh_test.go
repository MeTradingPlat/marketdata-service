package catchup

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type fakeVolumeProfileRepo struct {
	stale     []string
	computed  [][]string
	saved     map[string]domain.VolumeProfile
	attempted []string
	before    time.Time
}

func (f *fakeVolumeProfileRepo) GetSymbolsWithStaleVolumeProfile(context.Context, time.Time) ([]string, error) {
	return f.stale, nil
}

func (f *fakeVolumeProfileRepo) ComputeProfiles(_ context.Context, symbols []string, before time.Time) (map[string]domain.VolumeProfile, error) {
	f.computed = append(f.computed, symbols)
	f.before = before
	result := map[string]domain.VolumeProfile{}
	for _, symbol := range symbols {
		if symbol != "SIN_DATOS" {
			result[symbol] = domain.VolumeProfile{Symbol: symbol, Sessions: 5}
		}
	}
	return result, nil
}

func (f *fakeVolumeProfileRepo) SaveBatch(_ context.Context, profiles map[string]domain.VolumeProfile, attemptedOnly []string) error {
	if f.saved == nil {
		f.saved = map[string]domain.VolumeProfile{}
	}
	for symbol, profile := range profiles {
		f.saved[symbol] = profile
	}
	f.attempted = append(f.attempted, attemptedOnly...)
	return nil
}

func (f *fakeVolumeProfileRepo) GetBatch(context.Context, []string) (map[string]domain.VolumeProfile, error) {
	return nil, nil
}

func TestRefreshVolumeProfile_CalculaSoloLosVencidosEnLotesYMarcaLosSinDatos(t *testing.T) {
	stale := make([]string, 0, 450)
	for i := 0; i < 449; i++ {
		stale = append(stale, "S"+time.Duration(i).String())
	}
	stale = append(stale, "SIN_DATOS")
	repo := &fakeVolumeProfileRepo{stale: stale}

	err := RefreshVolumeProfile(context.Background(), repo, time.Now(), time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.computed) != 3 || len(repo.computed[0]) != volumeProfileBatchSize {
		t.Fatalf("lotes = %d, want 3 de %d", len(repo.computed), volumeProfileBatchSize)
	}
	if len(repo.saved) != 449 || len(repo.attempted) != 1 || repo.attempted[0] != "SIN_DATOS" {
		t.Fatalf("saved=%d attempted=%v", len(repo.saved), repo.attempted)
	}
}

func TestRefreshVolumeProfile_ExcluyeLaSesionEnCursoUsandoMedianocheDeNuevaYork(t *testing.T) {
	repo := &fakeVolumeProfileRepo{stale: []string{"AAPL"}}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("sin tzdata: %v", err)
	}

	_ = RefreshVolumeProfile(context.Background(), repo, time.Now(), time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC))

	want := time.Date(2026, 9, 20, 0, 0, 0, 0, loc)
	if !repo.before.Equal(want) {
		t.Fatalf("before = %v, want %v", repo.before, want)
	}
}

func TestRefreshVolumeProfile_SinVencidosNoHaceNada(t *testing.T) {
	repo := &fakeVolumeProfileRepo{}

	_ = RefreshVolumeProfile(context.Background(), repo, time.Now(), time.Now())

	if len(repo.computed) != 0 {
		t.Fatal("no debe calcular nada si no hay simbolos vencidos")
	}
}
