package domain

import "regexp"

var (
	securityTypePreferredRe      = regexp.MustCompile(`\bpreferred (stocks?|shares?|units?|limited)\b|\btrust preferred\b|\bpreferred securit|\bpreference\b`)
	securityTypePfdRe            = regexp.MustCompile(`\bpfd\b`)
	securityTypePreferredWordRe  = regexp.MustCompile(`\bpreferred\b`)
	securityTypeDepositaryRe     = regexp.MustCompile(`\bdep(ositary)? sh`)
	securityTypeDepositaryRcptRe = regexp.MustCompile(`\bdepositary receipts?\b`)
	securityTypeCommonShareRe    = regexp.MustCompile(`\bcommon (stock|shares)\b`)
	securityTypeFundRe           = regexp.MustCompile(`\bfunds?\b`)
	securityTypePreferredSymRe   = regexp.MustCompile(`^[A-Z]{1,5}p[A-Z]$`)
	securityTypeDebtOrDerivRe    = regexp.MustCompile(`\bnotes?\b|\bdebentures?\b|\bbonds?\b|\bunits?\b|\bwarrants?\b|\brights?\b`)
)

// isPreferredDescription reconoce preferreds por descripcion. "Preference"
// cubre emisiones como las de Triton o los "Trust Preference Securities" de
// SCE; "Preferred Limited" cubre los MLP ("Preferred Limited Partnership
// Units"). El guard de common stock/shares descarta fondos cerrados cuyo
// nombre menciona "Preferred Securities" pero que cotizan acciones comunes
// (confirmado: PTA, "Preferred Securities and Income Fund Common Shares").
// "Pfd" a secas no basta si la descripcion es el nombre de un fondo (HPF,
// "John Hancock Pfd Income Fund").
func isPreferredDescription(d string) bool {
	if securityTypeCommonShareRe.MatchString(d) {
		return false
	}
	if securityTypePreferredRe.MatchString(d) || isPreferredDepositaryShare(d) {
		return true
	}
	return securityTypePfdRe.MatchString(d) && !securityTypeFundRe.MatchString(d)
}

// isPreferredDepositaryShare: "Depositary Shares" a secas es la unica pista
// de muchos preferreds (TFCpI, METpE, SCEpL...). Los ADR/GDS ("American"/
// "Global Depositary Shares") ya se descartaron antes de llegar aca; los
// recibos depositarios se excluyen explicitamente.
func isPreferredDepositaryShare(d string) bool {
	return securityTypeDepositaryRe.MatchString(d) && !securityTypeDepositaryRcptRe.MatchString(d)
}

// isFallbackPreferred: cuando la descripcion no basta, la convencion de
// simbolos de TastyTrade (base en mayusculas + p + serie: BACpE, CpN,
// TRTNpA) delata los preferreds. Se exige que la descripcion no hable de
// deuda ni derivados porque la misma convencion la usan debentures (WRBpE),
// notes (MERpK) y units (NEEpT). "Preferred" a secas solo cuenta fuera de
// nombres de fondo (HPS, PFBC): el resto de los casos ya lo cubre la
// descripcion.
func isFallbackPreferred(symbol, d string) bool {
	if securityTypePreferredSymRe.MatchString(symbol) && !securityTypeDebtOrDerivRe.MatchString(d) {
		return true
	}
	return securityTypePreferredWordRe.MatchString(d) &&
		!securityTypeFundRe.MatchString(d) && !securityTypeCommonShareRe.MatchString(d)
}
