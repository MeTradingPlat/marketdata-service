package secedgar

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type candidateFiling struct {
	accessionNumber string
	primaryDocument string
	filingDate      time.Time
}

type secSubmissions struct {
	Filings struct {
		Recent struct {
			Form            []string `json:"form"`
			AccessionNumber []string `json:"accessionNumber"`
			FilingDate      []string `json:"filingDate"`
			PrimaryDocument []string `json:"primaryDocument"`
		} `json:"recent"`
	} `json:"filings"`
}

func findCandidateFilings(ctx context.Context, issuerCik int) ([]candidateFiling, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := fetchBody(fetchCtx, fmt.Sprintf(submissionsURLTemplate, issuerCik))
	if err != nil {
		return nil, err
	}
	var subs secSubmissions
	if err := json.Unmarshal(body, &subs); err != nil {
		return nil, fmt.Errorf("parsing sec submissions: %w", err)
	}

	recent := subs.Filings.Recent
	var candidates []candidateFiling
	for i, form := range recent.Form {
		if !hasRelevantPrefix(form) {
			continue
		}
		// EDGAR reporta esto como "xslSCHEDULE_13G_X01/primary_doc.xml"
		// (path del visor XSLT) pero el XML crudo siempre vive en la raiz
		// del accession como "primary_doc.xml" -- verificado en Java contra
		// filings reales. Filings pre-mandato (dic 2024) no terminan en este
		// nombre, quedan afuera a proposito (sin XML estructurado que
		// parsear).
		if i >= len(recent.PrimaryDocument) || !strings.HasSuffix(recent.PrimaryDocument[i], "primary_doc.xml") {
			continue
		}
		filingDate, err := time.Parse("2006-01-02", recent.FilingDate[i])
		if err != nil {
			continue
		}
		candidates = append(candidates, candidateFiling{
			accessionNumber: recent.AccessionNumber[i],
			primaryDocument: "primary_doc.xml",
			filingDate:      filingDate,
		})
	}
	return candidates, nil
}

func hasRelevantPrefix(form string) bool {
	for _, p := range relevantFormPrefixes {
		if strings.HasPrefix(form, p) {
			return true
		}
	}
	return false
}
