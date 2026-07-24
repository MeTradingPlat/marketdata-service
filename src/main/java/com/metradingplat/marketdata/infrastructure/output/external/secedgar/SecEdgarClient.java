package com.metradingplat.marketdata.infrastructure.output.external.secedgar;

import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import lombok.extern.slf4j.Slf4j;

/**
 * Free, no-key shares-outstanding fallback for symbols TastyTrade doesn't
 * cover (OTC/penny stocks, thin small-caps). Streams SEC's nightly bulk
 * companyfacts.zip entry-by-entry instead of buffering it whole, so peak
 * memory stays bounded by one company's JSON at a time, not the ~1.5GB
 * archive.
 */
@Slf4j
@Component
public class SecEdgarClient {

    private static final String USER_AGENT = "MeTradingPlat contrerasdaniel142@gmail.com";
    private static final String TICKERS_URL = "https://www.sec.gov/files/company_tickers.json";
    private static final String BULK_FACTS_URL = "https://www.sec.gov/Archives/edgar/daily-index/xbrl/companyfacts.zip";

    private final HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();
    private final ObjectMapper objectMapper = new ObjectMapper();
    private Map<String, Integer> tickerToCik;

    public Map<String, Long> fetchSharesOutstanding(List<String> symbols) {
        Map<String, String> cikToSymbol = new HashMap<>();
        for (String symbol : symbols) {
            Integer cik = tickerToCikMap().get(symbol.toUpperCase());
            if (cik != null) cikToSymbol.put(String.format("%010d", cik), symbol.toUpperCase());
        }
        if (cikToSymbol.isEmpty()) return Map.of();

        Map<String, Long> result = new HashMap<>();
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(BULK_FACTS_URL))
                .header("User-Agent", USER_AGENT)
                .timeout(Duration.ofMinutes(20))
                .GET()
                .build();
        try {
            HttpResponse<InputStream> response = httpClient.send(request, HttpResponse.BodyHandlers.ofInputStream());
            try (ZipInputStream zip = new ZipInputStream(response.body())) {
                ZipEntry entry;
                while (result.size() < cikToSymbol.size() && (entry = zip.getNextEntry()) != null) {
                    String cik = extractCik(entry.getName());
                    String symbol = cik != null ? cikToSymbol.get(cik) : null;
                    if (symbol != null) {
                        Long shares = parseSharesOutstanding(zip.readAllBytes());
                        if (shares != null) result.put(symbol, shares);
                    }
                    zip.closeEntry();
                }
            }
        } catch (Exception e) {
            log.warn("SEC EDGAR bulk download failed: {}", e.getMessage());
        }
        log.info("SEC EDGAR: fetched sharesOutstanding for {}/{} symbols", result.size(), symbols.size());
        return result;
    }

    private String extractCik(String entryName) {
        if (entryName == null || !entryName.startsWith("CIK") || !entryName.endsWith(".json")) return null;
        return entryName.substring(3, entryName.length() - 5);
    }

    private Long parseSharesOutstanding(byte[] entryData) {
        try {
            JsonNode shares = objectMapper.readTree(entryData)
                    .path("facts").path("dei").path("EntityCommonStockSharesOutstanding").path("units").path("shares");
            long latestVal = 0;
            String latestFiled = "";
            for (JsonNode entry : shares) {
                String filed = entry.path("filed").asText("");
                if (filed.compareTo(latestFiled) > 0) {
                    latestFiled = filed;
                    latestVal = entry.path("val").asLong(0);
                }
            }
            return latestVal > 0 ? latestVal : null;
        } catch (Exception e) {
            return null;
        }
    }

    private synchronized Map<String, Integer> tickerToCikMap() {
        if (tickerToCik != null) return tickerToCik;
        try {
            Map<String, Integer> map = new HashMap<>();
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create(TICKERS_URL))
                    .header("User-Agent", USER_AGENT)
                    .timeout(Duration.ofSeconds(15))
                    .GET()
                    .build();
            HttpResponse<InputStream> response = httpClient.send(request, HttpResponse.BodyHandlers.ofInputStream());
            JsonNode root = objectMapper.readTree(response.body());
            for (JsonNode entry : root) {
                String ticker = entry.path("ticker").asText(null);
                int cik = entry.path("cik_str").asInt(-1);
                if (ticker != null && cik > 0) map.put(ticker.toUpperCase(), cik);
            }
            log.info("SEC EDGAR: loaded {} ticker->CIK mappings", map.size());
            tickerToCik = map;
            return map;
        } catch (Exception e) {
            log.warn("SEC EDGAR: failed to load ticker->CIK mapping, will retry next run: {}", e.getMessage());
            return Map.of();
        }
    }
}
