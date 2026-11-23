package secedgar

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"
)

func TestFramesResponseDecodesDecimalVal(t *testing.T) {
	end := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	tests := []struct {
		name    string
		body    string
		wantVal float64
	}{
		{
			name:    "integer val",
			body:    fmt.Sprintf(`{"data":[{"cik":3333333333,"entityName":"Valid Issuer","val":%s,"end":"%s"}]}`, "34000000", end),
			wantVal: 34000000,
		},
		{
			name:    "decimal val",
			body:    fmt.Sprintf(`{"data":[{"cik":3333333333,"entityName":"Valid Issuer","val":%s,"end":"%s"}]}`, "51580911.816", end),
			wantVal: 51580911.816,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp framesResponse
			err := json.Unmarshal([]byte(tt.body), &resp)
			if err != nil {
				t.Fatalf("unexpected unmarshal error: %v", err)
			}

			shares := map[string]int64{}
			resolved := map[string]bool{}
			matchFrameEntities(resp.Data, map[string][]string{"3333333333": {"VLD"}}, resolved, shares)

			if shares["VLD"] != int64(math.Round(tt.wantVal)) {
				t.Errorf("shares[VLD] = %d, want %d", shares["VLD"], int64(math.Round(tt.wantVal)))
			}
			if resolved["3333333333"] != true {
				t.Errorf("resolved[3333333333] = %v, want true", resolved["3333333333"])
			}
		})
	}
}
