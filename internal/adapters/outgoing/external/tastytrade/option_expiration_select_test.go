package tastytrade

import (
	"testing"
	"time"
)

func TestFrontMonthlyExpiration(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	oneStrike := []optionStrike{{Call: "C1", Put: "P1"}}

	tests := []struct {
		name        string
		expirations []optionExpiration
		wantDate    string
		wantOK      bool
	}{
		{
			name: "prefiere Regular sobre weekly mas cercano",
			expirations: []optionExpiration{
				{ExpirationDate: "2026-11-27", ExpirationType: "Weekly", Strikes: oneStrike},
				{ExpirationDate: "2026-12-18", ExpirationType: "Regular", Strikes: oneStrike},
			},
			wantDate: "2026-12-18",
			wantOK:   true,
		},
		{
			name: "elige el Regular futuro mas cercano entre varios",
			expirations: []optionExpiration{
				{ExpirationDate: "2027-01-15", ExpirationType: "Regular", Strikes: oneStrike},
				{ExpirationDate: "2026-12-18", ExpirationType: "Regular", Strikes: oneStrike},
				{ExpirationDate: "2027-02-19", ExpirationType: "regular", Strikes: oneStrike},
			},
			wantDate: "2026-12-18",
			wantOK:   true,
		},
		{
			name: "ignora Regular pasado",
			expirations: []optionExpiration{
				{ExpirationDate: "2026-11-20", ExpirationType: "Regular", Strikes: oneStrike},
				{ExpirationDate: "2026-11-24", ExpirationType: "Regular", Strikes: oneStrike},
				{ExpirationDate: "2026-12-18", ExpirationType: "Regular", Strikes: oneStrike},
			},
			wantDate: "2026-12-18",
			wantOK:   true,
		},
		{
			name: "cae al futuro mas cercano sin Regular",
			expirations: []optionExpiration{
				{ExpirationDate: "2026-12-04", ExpirationType: "Weekly", Strikes: oneStrike},
				{ExpirationDate: "2026-11-27", ExpirationType: "Weekly", Strikes: oneStrike},
				{ExpirationDate: "2026-12-31", ExpirationType: "Quarterly", Strikes: oneStrike},
			},
			wantDate: "2026-11-27",
			wantOK:   true,
		},
		{
			name: "ignora expiraciones sin strikes o con fecha invalida",
			expirations: []optionExpiration{
				{ExpirationDate: "not-a-date", ExpirationType: "Regular", Strikes: oneStrike},
				{ExpirationDate: "2026-11-27", ExpirationType: "Regular", Strikes: nil},
				{ExpirationDate: "2026-12-18", ExpirationType: "Regular", Strikes: oneStrike},
			},
			wantDate: "2026-12-18",
			wantOK:   true,
		},
		{
			name: "devuelve false si todas estan en el pasado",
			expirations: []optionExpiration{
				{ExpirationDate: "2026-11-20", ExpirationType: "Regular", Strikes: oneStrike},
				{ExpirationDate: "2026-11-24", ExpirationType: "Weekly", Strikes: oneStrike},
			},
			wantOK: false,
		},
		{
			name:        "devuelve false con lista vacia",
			expirations: nil,
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := frontMonthlyExpiration(tt.expirations, now)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %+v)", ok, tt.wantOK, got)
			}
			if tt.wantOK && got.ExpirationDate != tt.wantDate {
				t.Errorf("ExpirationDate = %q, want %q", got.ExpirationDate, tt.wantDate)
			}
		})
	}
}
