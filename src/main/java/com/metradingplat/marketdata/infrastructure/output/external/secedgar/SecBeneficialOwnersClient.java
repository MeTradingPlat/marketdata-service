package com.metradingplat.marketdata.infrastructure.output.external.secedgar;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import javax.xml.parsers.DocumentBuilder;
import javax.xml.parsers.DocumentBuilderFactory;

import org.springframework.stereotype.Component;
import org.w3c.dom.Document;
import org.w3c.dom.Element;
import org.w3c.dom.NodeList;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Holders institucionales de 5%+ (Schedule 13D/13G) no tienen archivo bulk
 * de la SEC -- a diferencia de FinraClient/SecEdgarClient/SecInsiderOwnershipClient,
 * esto se pide por simbolo bajo demanda. Solo cubre filings posteriores al
 * mandato de diciembre 2024 (portada en XML estructurado); los anteriores
 * quedan omitidos a proposito para no depender de scraping de HTML.
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class SecBeneficialOwnersClient {

    private static final String USER_AGENT = SecTickerCikLookup.USER_AGENT;
    private static final String SUBMISSIONS_URL = "https://data.sec.gov/submissions/CIK%010d.json";
    private static final String DOC_URL = "https://www.sec.gov/Archives/edgar/data/%d/%s/%s";
    private static final List<String> RELEVANT_FORM_PREFIXES =
            List.of("SC 13D", "SC 13G", "SCHEDULE 13D", "SCHEDULE 13G");

    private final SecTickerCikLookup tickerCikLookup;
    private final HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();
    private final ObjectMapper objectMapper = new ObjectMapper();

    public long fetchBeneficialOwnerShares(String symbol, Set<String> excludeCiks) {
        Integer issuerCik = tickerCikLookup.tickerToCikMap().get(symbol.toUpperCase());
        if (issuerCik == null) return 0L;

        try {
            List<CandidateFiling> candidates = findCandidateFilings(issuerCik);
            return sumLatestPerFiler(issuerCik, candidates, excludeCiks);
        } catch (Exception e) {
            log.debug("SEC beneficial owners lookup failed for {}: {}", symbol, e.getMessage());
            return 0L;
        }
    }

    private record CandidateFiling(String accessionNumber, String primaryDocument, LocalDate filingDate) { }

    private List<CandidateFiling> findCandidateFilings(int issuerCik) throws Exception {
        JsonNode root = fetchJson(String.format(SUBMISSIONS_URL, issuerCik));
        JsonNode recent = root.path("filings").path("recent");
        JsonNode forms = recent.path("form");
        JsonNode accessionNumbers = recent.path("accessionNumber");
        JsonNode filingDates = recent.path("filingDate");
        JsonNode primaryDocuments = recent.path("primaryDocument");

        List<CandidateFiling> candidates = new ArrayList<>();
        for (int i = 0; i < forms.size(); i++) {
            String form = forms.path(i).asText("");
            if (RELEVANT_FORM_PREFIXES.stream().noneMatch(form::startsWith)) continue;
            String primaryDoc = primaryDocuments.path(i).asText("");
            // EDGAR reporta esto como "xslSCHEDULE_13G_X01/primary_doc.xml" (path
            // del visor XSLT) pero el XML crudo siempre vive en la raiz del
            // accession como "primary_doc.xml" -- verificado contra filings
            // reales. Filings pre-mandato (dic 2024) no terminan en este nombre,
            // quedan afuera a proposito (sin XML estructurado que parsear).
            if (!primaryDoc.endsWith("primary_doc.xml")) continue;
            candidates.add(new CandidateFiling(
                    accessionNumbers.path(i).asText(""),
                    "primary_doc.xml",
                    LocalDate.parse(filingDates.path(i).asText())));
        }
        return candidates;
    }

    private record FilerHolding(String filerCik, LocalDate filingDate, long shares) { }

    private long sumLatestPerFiler(int issuerCik, List<CandidateFiling> candidates, Set<String> excludeCiks) {
        Map<String, FilerHolding> latestByFiler = new HashMap<>();
        for (CandidateFiling candidate : candidates) {
            FilerHolding holding = fetchFilerHolding(issuerCik, candidate);
            if (holding == null || excludeCiks.contains(holding.filerCik())) continue;
            FilerHolding current = latestByFiler.get(holding.filerCik());
            if (current == null || holding.filingDate().isAfter(current.filingDate())) {
                latestByFiler.put(holding.filerCik(), holding);
            }
        }
        long total = 0L;
        for (FilerHolding holding : latestByFiler.values()) total += holding.shares();
        return total;
    }

    private FilerHolding fetchFilerHolding(int issuerCik, CandidateFiling candidate) {
        try {
            String accessionNoDashes = candidate.accessionNumber().replace("-", "");
            String url = String.format(DOC_URL, issuerCik, accessionNoDashes, candidate.primaryDocument());
            Document doc = fetchXml(url);

            String filerCik = textContent(doc, "cik");
            NodeList reportingPersons = doc.getElementsByTagName("coverPageHeaderReportingPersonDetails");
            if (filerCik == null || reportingPersons.getLength() == 0) return null;

            // Un mismo filing repite la misma posicion en varios bloques
            // (fondo, su gestora, la persona que la controla) -- se toma
            // solo el primero, sumar todos triplicaria el conteo.
            Element firstBlock = (Element) reportingPersons.item(0);
            long shares = parseShares(textContent(firstBlock, "reportingPersonBeneficiallyOwnedAggregateNumberOfShares"));
            return new FilerHolding(filerCik, candidate.filingDate(), shares);
        } catch (Exception e) {
            log.debug("Failed to parse beneficial ownership filing {}: {}", candidate.accessionNumber(), e.getMessage());
            return null;
        }
    }

    private long parseShares(String value) {
        if (value == null || value.isBlank()) return 0L;
        try { return (long) Double.parseDouble(value); }
        catch (NumberFormatException e) { return 0L; }
    }

    private String textContent(Document doc, String tagName) {
        NodeList nodes = doc.getElementsByTagName(tagName);
        return nodes.getLength() > 0 ? nodes.item(0).getTextContent().trim() : null;
    }

    private String textContent(Element element, String tagName) {
        NodeList nodes = element.getElementsByTagName(tagName);
        return nodes.getLength() > 0 ? nodes.item(0).getTextContent().trim() : null;
    }

    private JsonNode fetchJson(String url) throws Exception {
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .header("User-Agent", USER_AGENT)
                .timeout(Duration.ofSeconds(15))
                .GET()
                .build();
        HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        return objectMapper.readTree(response.body());
    }

    private Document fetchXml(String url) throws Exception {
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .header("User-Agent", USER_AGENT)
                .timeout(Duration.ofSeconds(15))
                .GET()
                .build();
        HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());

        DocumentBuilderFactory factory = DocumentBuilderFactory.newInstance();
        factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
        factory.setXIncludeAware(false);
        factory.setExpandEntityReferences(false);
        DocumentBuilder builder = factory.newDocumentBuilder();
        return builder.parse(new org.xml.sax.InputSource(new java.io.StringReader(response.body())));
    }
}
