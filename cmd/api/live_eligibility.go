package main

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type channelPacker interface {
	SetMaxChannelsPerConn(n int)
}

func liveEligible(symbols []domain.Symbol) []domain.Symbol {
	var eligible []domain.Symbol
	for _, s := range symbols {
		st := domain.ClassifySecurityType(s.Symbol, s.IsEtf, s.Description)
		if st == "WARRANT" || st == "UNIT" || st == "RIGHTS" {
			continue
		}
		eligible = append(eligible, s)
	}
	return eligible
}
