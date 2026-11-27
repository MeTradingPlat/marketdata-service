package domain

import (
	"regexp"
	"strings"
)

var tradedUnitRe = regexp.MustCompile(`\bcommon units\b|\blimited partner(?:s|ship)?\b|\bl\.p\. units\b|\btrust units\b|\bunits of beneficial interest\b|\bcorporate units\b|\btangible equity units?\b|\bclass a units\b`)

func IsTradedUnit(description string) bool {
	return tradedUnitRe.MatchString(strings.ToLower(strings.TrimSpace(description)))
}
