package secedgar

import (
	"encoding/json"
	"time"
)

// minPlausibleShares: menos de 10k acciones es casi siempre un desglose por
// clase de accion (ej. la clase B del fundador) filtrado por error, no el
// total de la empresa -- confirmado en Java con FNKO, cuyo unico dato en
// us-gaap:CommonStockSharesOutstanding eran 100 acciones de 2017 (Class B),
// mientras el total real es ~34M.
const minPlausibleShares = 10_000

// maxStaleness: si el dato mas reciente disponible tiene mas de esto, se
// prefiere no tener dato (nil, cae al heuristico/DxLink) antes que mostrar
// una cifra que la empresa dejo de reportar bajo ese tag hace años.
const maxStaleness = 2 * 365 * 24 * time.Hour

type secFactUnit struct {
	Val   int64  `json:"val"`
	Filed string `json:"filed"`
	End   string `json:"end"`
}

type secFacts struct {
	Facts struct {
		Dei struct {
			EntityCommonStockSharesOutstanding struct {
				Units struct {
					Shares []secFactUnit `json:"shares"`
				} `json:"units"`
			} `json:"EntityCommonStockSharesOutstanding"`
		} `json:"dei"`
		UsGaap struct {
			CommonStockSharesOutstanding struct {
				Units struct {
					Shares []secFactUnit `json:"shares"`
				} `json:"units"`
			} `json:"CommonStockSharesOutstanding"`
		} `json:"us-gaap"`
	} `json:"facts"`
}

func parseSharesOutstanding(data []byte) *int64 {
	var facts secFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		return nil
	}
	if shares := latestShareCount(facts.Facts.Dei.EntityCommonStockSharesOutstanding.Units.Shares); shares != nil {
		return shares
	}
	return latestShareCount(facts.Facts.UsGaap.CommonStockSharesOutstanding.Units.Shares)
}

func latestShareCount(units []secFactUnit) *int64 {
	var latestVal int64
	var latestFiled, latestEnd string
	for _, u := range units {
		if u.Filed > latestFiled {
			latestFiled, latestVal, latestEnd = u.Filed, u.Val, u.End
		}
	}
	if latestVal < minPlausibleShares || isStale(latestEnd) {
		return nil
	}
	return &latestVal
}

func isStale(endDate string) bool {
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return true
	}
	return end.Before(time.Now().Add(-maxStaleness))
}
