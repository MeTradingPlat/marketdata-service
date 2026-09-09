package numparse

import (
	"strconv"
	"strings"
)

// Float parsea un numero tolerando espacios y valores invalidos -- las APIs
// externas (TastyTrade, FINRA) devuelven campos numericos como string, a
// veces con espacios de por medio, y "no se pudo parsear" en estos casos
// significa "dato ausente", no un error a propagar.
func Float(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return f
}

// Int es el equivalente de Float para enteros.
func Int(v string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
