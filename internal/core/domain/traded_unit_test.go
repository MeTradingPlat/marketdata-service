package domain

import "testing"

func TestIsTradedUnit(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want bool
	}{
		{"common units", "Energy Transfer LP Common Units", true},
		{"limited partnership units", "Brookfield Renewable Partners L.P. Limited Partnership Units", true},
		{"limited partner interests", "Icahn Enterprises L.P. - Depositary Units representing Limited Partner Interests", true},
		{"l.p. units", "AllianceBernstein Holding L.P. Units", true},
		{"trust units", "Sprott Physical Gold and Silver Trust Units", true},
		{"units of beneficial interest", "PermRock Royalty Trust Units of Beneficial Interest", true},
		{"corporate units", "NextEra Energy, Inc. 7.375% Corporate Units", true},
		{"tangible equity unit", "BrightSpring Health Services, Inc. - Tangible Equity Unit", true},
		{"class a units", "Belpointe PREP, LLC Class A Units", true},
		{"assignments of limited partnership", "Greystone Housing Impact Investors LP Beneficial Unit Certificates representing assignments of limited partnership interests", true},
		{"spac gores units", "Gores Holdings X, Inc. - Units", false},
		{"spac archimedes unit", "Archimedes Tech SPAC Partners III Co. - Unit", false},
		{"spac karbon units", "Karbon Capital Partners Corp. - Units", false},
		{"spac legato units", "Legato Merger Corp. IV Units, each consisting of one ordinary share and one-third of one redeemable warrant", false},
		{"spac football manager", "Football Manager Spac Inc Units (1 Class A & 1 War)", false},
		{"spac thunder bridge", "Thunder Bridge Capital Partners V Ltd Units (1 Class A & 1/5 War)", false},
		{"market tech aq unit", "MARKET TECH AQ UNIT", false},
		{"purecycle unit", "PureCycle Technologies, Inc. - Unit", false},
		{"empty description", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTradedUnit(tt.desc); got != tt.want {
				t.Errorf("IsTradedUnit(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}
