package domain

import (
	"testing"
	"time"
)

func TestNYSECalendar_2026To2028(t *testing.T) {
	holidays := map[string]bool{
		"2026-01-01": true, "2026-01-19": true, "2026-02-16": true, "2026-04-03": true, "2026-05-25": true,
		"2026-06-19": true, "2026-07-03": true, "2026-09-07": true, "2026-11-26": true, "2026-12-25": true,
		"2027-01-01": true, "2027-01-18": true, "2027-02-15": true, "2027-03-26": true, "2027-05-31": true,
		"2027-06-18": true, "2027-07-05": true, "2027-09-06": true, "2027-11-25": true, "2027-12-24": true,
		"2028-01-17": true, "2028-02-21": true, "2028-04-14": true, "2028-05-29": true, "2028-06-19": true,
		"2028-07-04": true, "2028-09-04": true, "2028-11-23": true, "2028-12-25": true,
	}
	early := map[string]bool{
		"2026-11-27": true, "2026-12-24": true,
		"2027-11-26": true,
		"2028-07-03": true, "2028-11-24": true,
	}
	end := time.Date(2028, 12, 31, 0, 0, 0, 0, time.UTC)
	for d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if got := IsNYSEHoliday(d); got != holidays[key] {
			t.Fatalf("IsNYSEHoliday(%s) = %v, want %v", key, got, holidays[key])
		}
		if got := IsNYSEEarlyClose(d); got != early[key] {
			t.Fatalf("IsNYSEEarlyClose(%s) = %v, want %v", key, got, early[key])
		}
	}
}
