package metadata

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

func TestApplyExternalFundamentals(t *testing.T) {
	now := time.Now()
	shares, flt, shortShares := int64(1000), int64(500), int64(50)
	legacy, ratio := 8.5, 1.5

	var nilOut dto.FundamentalData
	applyExternalFundamentals(&nilOut, domain.Fundamentals{SharesOutstanding: &shares})
	if nilOut.SharesOutstanding != nil || nilOut.FloatSource != nil {
		t.Fatalf("expected no fields set when ExternalUpdatedAt is nil, got %+v", nilOut)
	}

	var secOut dto.FundamentalData
	applyExternalFundamentals(&secOut, domain.Fundamentals{
		ExternalUpdatedAt: &now, FloatUpdatedAt: &now,
		SharesOutstanding: &shares, FloatShares: &flt,
		ShortInterestShares: &shortShares, ShortRatio: &ratio,
	})
	if secOut.ShortInterest == nil || *secOut.ShortInterest != 10 || secOut.FloatSource == nil || *secOut.FloatSource != "SEC_EDGAR" {
		t.Fatalf("SEC_EDGAR case mismatch: %+v", secOut)
	}

	var estOut dto.FundamentalData
	applyExternalFundamentals(&estOut, domain.Fundamentals{
		ExternalUpdatedAt: &now, FloatShares: &flt, ShortInterest: &legacy,
	})
	if estOut.ShortInterest == nil || *estOut.ShortInterest != 8.5 || estOut.FloatSource == nil || *estOut.FloatSource != "ESTIMATED" {
		t.Fatalf("ESTIMATED case mismatch: %+v", estOut)
	}
}
