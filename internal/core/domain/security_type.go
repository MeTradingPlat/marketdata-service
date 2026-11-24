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
	securityTypeADRRe    = regexp.MustCompile(`\bamerican depositary|\bglobal depositary|\badrs?\b`)
)

// ClassifySecurityType es la misma heuristica de clasificacion que ya
// probo el servicio Java contra el universo real de TastyTrade -- symbol y
// description ya los tenemos (Symbol), no hace falta pedirle nada mas a
// nadie para calcularlo.
//
// Los ADR/GDS se clasifican EQUITY antes que nada: su descripcion dice
// "American Depositary Shares" y la regla de depositary shares los marcaba
// PREFERRED (confirmado en el universo real: 277 falsos positivos, entre
// ellos todos los ADR y los GDS de IRSA).
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
	if securityTypeADRRe.MatchString(d) {
		return "EQUITY"
	}
	if isPreferredDescription(d) {
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
	if isFallbackPreferred(symbol, d) {
		return "PREFERRED"
	}
	return "EQUITY"
}
