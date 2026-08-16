package secedgar

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// xmlNode es un arbol generico (cualquier tag, cualquier profundidad) --
// primary_doc.xml no tiene un schema fijo publico estable entre formularios,
// asi que buscamos por nombre de tag local en cualquier nivel (equivalente
// a Document.getElementsByTagName de la version Java) en vez de un struct
// con la forma exacta.
type xmlNode struct {
	XMLName xml.Name
	Content string    `xml:",chardata"`
	Nodes   []xmlNode `xml:",any"`
}

func findAllByTag(n xmlNode, tag string) []xmlNode {
	var result []xmlNode
	if n.XMLName.Local == tag {
		result = append(result, n)
	}
	for _, child := range n.Nodes {
		result = append(result, findAllByTag(child, tag)...)
	}
	return result
}

type filerHolding struct {
	filerCik   string
	filingDate time.Time
	shares     int64
}

func sumLatestPerFiler(ctx context.Context, issuerCik int, candidates []candidateFiling, excludeCiks map[string]bool) int64 {
	latestByFiler := make(map[string]filerHolding)
	for _, candidate := range candidates {
		holding, err := fetchFilerHolding(ctx, issuerCik, candidate)
		if err != nil || excludeCiks[holding.filerCik] {
			continue
		}
		current, exists := latestByFiler[holding.filerCik]
		if !exists || holding.filingDate.After(current.filingDate) {
			latestByFiler[holding.filerCik] = holding
		}
	}

	var total int64
	for _, h := range latestByFiler {
		total += h.shares
	}
	return total
}

func fetchFilerHolding(ctx context.Context, issuerCik int, candidate candidateFiling) (filerHolding, error) {
	accessionNoDashes := strings.ReplaceAll(candidate.accessionNumber, "-", "")
	url := fmt.Sprintf(beneficialOwnerDocURLTemplate, issuerCik, accessionNoDashes, candidate.primaryDocument)

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := fetchBody(fetchCtx, url)
	if err != nil {
		return filerHolding{}, err
	}

	var root xmlNode
	if err := xml.Unmarshal(body, &root); err != nil {
		return filerHolding{}, fmt.Errorf("parsing beneficial owner xml: %w", err)
	}

	ciks := findAllByTag(root, "cik")
	persons := findAllByTag(root, "coverPageHeaderReportingPersonDetails")
	if len(ciks) == 0 || len(persons) == 0 {
		return filerHolding{}, fmt.Errorf("no reporting person in filing %s", candidate.accessionNumber)
	}

	// Un mismo filing repite la misma posicion en varios bloques (fondo, su
	// gestora, la persona que la controla) -- se toma solo el primero, sumar
	// todos triplicaria el conteo.
	sharesNodes := findAllByTag(persons[0], "reportingPersonBeneficiallyOwnedAggregateNumberOfShares")
	if len(sharesNodes) == 0 {
		return filerHolding{}, fmt.Errorf("no shares field in filing %s", candidate.accessionNumber)
	}

	shares, _ := strconv.ParseFloat(strings.TrimSpace(sharesNodes[0].Content), 64)
	return filerHolding{
		filerCik:   strings.TrimSpace(ciks[0].Content),
		filingDate: candidate.filingDate,
		shares:     int64(shares),
	}, nil
}
