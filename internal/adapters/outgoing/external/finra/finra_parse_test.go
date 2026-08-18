package finra

import "testing"

// Header y filas reales del CSV descargado de
// https://cdn.finra.org/equity/otcmarket/biweekly/shrt20260731.csv -- el
// parser debe coincidir con el formato vigente de FINRA, no con una
// suposicion: symbol=col[1], currentShortPositionQuantity=col[5],
// averageDailyVolumeQuantity=col[8], daysToCoverQuantity=col[9],
// settlementDate=col[13].
const realFinraSample = `accountingYearMonthNumber|symbolCode|issueName|issuerServicesGroupExchangeCode|marketClassCode|currentShortPositionQuantity|previousShortPositionQuantity|stockSplitFlag|averageDailyVolumeQuantity|daysToCoverQuantity|revisionFlag|changePercent|changePreviousNumber|settlementDate
20260731|A|Agilent Technologies Inc.|A|NYSE|5749623|7538437||2301495|2.50||-23.73|-1788814|2026-07-31
20260731|AA|Alcoa Corporation|A|NYSE|9334029|8981785||5721292|1.63||3.92|352244|2026-07-31
20260731|AAA|Alternative Access First Priority CLO Bond ETF|B|ARCA|1000|5000||75000|0.01||-80.00|-4000|2026-07-31
`

func TestParseFinraCsvRealFormat(t *testing.T) {
	records := parseFinraCsv([]byte(realFinraSample))
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	a, ok := records["A"]
	if !ok {
		t.Fatal("expected symbol A in parsed records")
	}
	if a.SharesShorted != 5749623 {
		t.Errorf("A sharesShorted: expected 5749623, got %d", a.SharesShorted)
	}
	if a.AvgDailyVolume != 2301495 {
		t.Errorf("A avgDailyVolume: expected 2301495, got %d", a.AvgDailyVolume)
	}
	if a.DaysToCover != 2.50 {
		t.Errorf("A daysToCover: expected 2.50, got %v", a.DaysToCover)
	}
	if a.SettlementDate != "2026-07-31" {
		t.Errorf("A settlementDate: expected 2026-07-31, got %q", a.SettlementDate)
	}

	aaa, ok := records["AAA"]
	if !ok {
		t.Fatal("expected symbol AAA in parsed records")
	}
	if aaa.SharesShorted != 1000 {
		t.Errorf("AAA sharesShorted: expected 1000, got %d", aaa.SharesShorted)
	}
}

func TestParseFinraCsvRejectsErrorPage(t *testing.T) {
	if got := parseFinraCsv([]byte("<?xml version=\"1.0\"?><Error><Code>AccessDenied</Code></Error>")); got != nil {
		t.Errorf("expected nil for XML error page, got %d records", len(got))
	}
	if got := parseFinraCsv([]byte("")); got != nil {
		t.Errorf("expected nil for empty body, got %d records", len(got))
	}
}
