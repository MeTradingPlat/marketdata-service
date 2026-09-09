package domain

import "regexp"

// symbolFormatRe acepta los caracteres que de verdad aparecen en simbolos
// reales de TastyTrade/DxLink (letras, digitos, punto para clases de accion
// como BRK.B, guion) con un tope de longitud generoso -- no es una lista
// blanca de simbolos validos, solo un guard contra strings basura/gigantes
// llegando sin sanitizar hasta cache keys, la BD y DxLink.
var symbolFormatRe = regexp.MustCompile(`^[A-Za-z0-9.\-]{1,20}$`)

// ValidSymbolFormat valida forma, no existencia -- un simbolo bien formado
// pero inexistente sigue devolviendo "sin datos" mas adelante en el flujo,
// como siempre.
func ValidSymbolFormat(symbol string) bool {
	return symbolFormatRe.MatchString(symbol)
}
