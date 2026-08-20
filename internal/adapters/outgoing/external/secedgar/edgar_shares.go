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

// companyFacts es lo que se saca de UN pase sobre el JSON de una empresa --
// sharesOutstanding y la fecha de filing mas reciente comparten exactamente
// las mismas unidades (cover page de cada 10-Q/10-K), asi que se sacan
// juntas en la misma pasada en vez de volver a decodificar ~1.5GB de bulk
// dos veces (una por cada dato).
type companyFacts struct {
	Shares    *int64
	LastFiled string
}

func parseCompanyFacts(data []byte) companyFacts {
	var facts secFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		return companyFacts{}
	}
	dei := facts.Facts.Dei.EntityCommonStockSharesOutstanding.Units.Shares
	gaap := facts.Facts.UsGaap.CommonStockSharesOutstanding.Units.Shares

	shares := latestShareCount(dei)
	if shares == nil {
		shares = latestShareCount(gaap)
	}

	filed := latestFiledDate(dei)
	if f := latestFiledDate(gaap); f > filed {
		filed = f
	}
	return companyFacts{Shares: shares, LastFiled: filed}
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

// latestFiledDate: la fecha en que la SEC recibio el 10-Q/10-K mas reciente
// -- proxy razonable de "ultimo reporte de earnings" cuando TastyTrade
// todavia no tiene el reporte real en su historial (confirmado en vivo con
// INTC: TastyTrade seguia devolviendo el cierre de trimestre como
// placeholder semanas despues del reporte real). No aplica el mismo filtro
// de plausibilidad que shares -- una fecha vieja simplemente pierde contra
// isStale() al usarse, no hace falta descartarla aca.
func latestFiledDate(units []secFactUnit) string {
	var latest string
	for _, u := range units {
		if u.Filed > latest {
			latest = u.Filed
		}
	}
	return latest
}

func isStale(endDate string) bool {
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return true
	}
	return end.Before(time.Now().Add(-maxStaleness))
}
