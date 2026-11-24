package domain

import "testing"

func TestClassifySecurityType(t *testing.T) {
	tests := []struct {
		name        string
		symbol      string
		isEtf       bool
		description string
		want        string
	}{
		{
			name:        "ADR American Depositary Shares es EQUITY",
			symbol:      "BABA",
			description: "Alibaba Group Holding Limited American Depositary Shares each representing eight Ordinary shares",
			want:        "EQUITY",
		},
		{
			name:        "GDS Global Depositary Shares es EQUITY",
			symbol:      "IRS",
			description: "IRSA Inversiones Y Representaciones S.A. Global Depositary Shares (Each representing ten Class B shares)",
			want:        "EQUITY",
		},
		{
			name:        "ADR de preferentes extranjeras es EQUITY",
			symbol:      "PBR/A",
			description: "Petroleo Brasileiro S.A. Petrobras American Depositary Shares representing Preferred Shares",
			want:        "EQUITY",
		},
		{
			name:        "preferred depositary shares",
			symbol:      "TFCpI",
			description: "Truist Financial Corporation Depositary Shares",
			want:        "PREFERRED",
		},
		{
			name:        "preferred por simbolo sin descripcion explicita",
			symbol:      "SCEpL",
			description: "SCE TRUST VI",
			want:        "PREFERRED",
		},
		{
			name:        "preferred units MLP",
			symbol:      "SEALpA",
			description: "Seapeak LLC 9.00% Series A Cumulative Redeemable Perpetual Preferred Units",
			want:        "PREFERRED",
		},
		{
			name:        "preference shares",
			symbol:      "TRTNpA",
			description: "Triton International Limited 8.50% Series A Cumulative Redeemable Perpetual Preference Shares",
			want:        "PREFERRED",
		},
		{
			name:        "depositary sh pfd",
			symbol:      "BACpE",
			description: "Bank of America Corporation Depositary Sh repstg 1/1000th Perp Pfd Ser E",
			want:        "PREFERRED",
		},
		{
			name:        "trust preferred securities",
			symbol:      "CpN",
			description: "Citigroup Capital XIII 7.875% Fixed rate Floating Rate trust Preferred Securities",
			want:        "PREFERRED",
		},
		{
			name:        "term preferred stock",
			symbol:      "PRIFpD",
			description: "Priority Income Fund, Inc. 7.00% Series D Term Preferred Stock due 2029",
			want:        "PREFERRED",
		},
		{
			name:        "fondo cerrado con Pfd en el nombre es EQUITY",
			symbol:      "HPF",
			description: "John Hancock Pfd Income Fund II Pfd Income Fund II",
			want:        "EQUITY",
		},
		{
			name:        "fondo cerrado con Preferred Securities y Common Shares es EQUITY",
			symbol:      "PTA",
			description: "Cohen & Steers Tax-Advantaged Preferred Securities and Income Fund Common Shares of Beneficial Interest",
			want:        "EQUITY",
		},
		{
			name:        "banco llamado Preferred Bank Common Stock es EQUITY",
			symbol:      "PFBC",
			description: "Preferred Bank - Common Stock",
			want:        "EQUITY",
		},
		{
			name:        "corporate units con p en simbolo es UNIT",
			symbol:      "NEEpT",
			description: "NextEra Energy, Inc. 7.234% Corporate Units",
			want:        "UNIT",
		},
		{
			name:        "subordinated debentures con p en simbolo es EQUITY",
			symbol:      "WRBpE",
			description: "W.R. Berkley Corporation 5.70% Subordinated Debentures due 2058",
			want:        "EQUITY",
		},
		{
			name:        "notes due con p en simbolo es BOND",
			symbol:      "FpB",
			description: "Ford Motor Company 6.20% Notes due June 1, 2059",
			want:        "BOND",
		},
		{
			name:        "warrant por sufijo /WS",
			symbol:      "ABC/WS",
			description: "ABC Corp Warrants",
			want:        "WARRANT",
		},
		{
			name:        "unit por sufijo /U",
			symbol:      "ABC/U",
			description: "ABC Corp Units",
			want:        "UNIT",
		},
		{
			name:        "rights por descripcion",
			symbol:      "ABC",
			description: "ABC Corp Rights to purchase common stock",
			want:        "RIGHTS",
		},
		{
			name:        "ETF por bandera isEtf",
			symbol:      "SPY",
			isEtf:       true,
			description: "SPDR S&P 500 ETF Trust",
			want:        "ETF",
		},
		{
			name:        "simbolo de prueba por nombre",
			symbol:      "TESTX",
			description: "anything",
			want:        "TEST_SYMBOL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifySecurityType(tt.symbol, tt.isEtf, tt.description)
			if got != tt.want {
				t.Errorf("ClassifySecurityType(%q, %v, %q) = %q, want %q", tt.symbol, tt.isEtf, tt.description, got, tt.want)
			}
		})
	}
}
