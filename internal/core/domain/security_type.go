package domain

import (
	"regexp"
	"strings"
)

var (
	securityTypeNotesRe  = regexp.MustCompile(`\bnotes? due\b|\b(senior|subordinated) notes\b`)
	securityTypeUnitRe   = regexp.MustCompile(`\bunits?\b`)
	securityTypeRightsRe = regexp.MustCompile(`\brights?\b`)
	securityTypeWarrRe   = regexp.MustCompile(`\bwarrants?\b`)
)

// ClassifySecurityType es la misma heuristica de clasificacion que ya
// probo el servicio Java contra el universo real de TastyTrade -- symbol y
// description ya los tenemos (Symbol), no hace falta pedirle nada mas a
// nadie para calcularlo.
func ClassifySecurityType(symbol string, isEtf bool, description string) string {
	if strings.Contains(symbol, "TEST") {
		return "TEST_SYMBOL"
	}
	d := strings.ToLower(strings.TrimSpace(description))
	if strings.Contains(d, "tick pilot") || strings.Contains(d, "symbology tst") {
		return "TEST_SYMBOL"
	}
	if isEtf || strings.Contains(d, "etf") {
		return "ETF"
	}
	if securityTypeNotesRe.MatchString(d) || strings.Contains(d, "mortgage bonds") {
		return "BOND"
	}
	if strings.Contains(d, "preferred stock") || strings.Contains(d, "preferred units") ||
		strings.Contains(d, "preferred shares") || strings.Contains(d, "depositary shares") {
		return "PREFERRED"
	}
	if strings.HasSuffix(symbol, "/WS") || securityTypeWarrRe.MatchString(d) {
		return "WARRANT"
	}
	if strings.HasSuffix(symbol, "/U") || securityTypeUnitRe.MatchString(d) {
		return "UNIT"
	}
	if securityTypeRightsRe.MatchString(d) {
		return "RIGHTS"
	}
	return "EQUITY"
}
