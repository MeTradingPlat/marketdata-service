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

import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import lombok.extern.slf4j.Slf4j;

/**
 * Free, no-key shares-outstanding fallback for symbols TastyTrade doesn't cover
 * (OTC/penny stocks, thin small-caps). SEC requires a real User-Agent and a
 * max of 10 req/sec, hence the throttling between per-company lookups.
 */
@Slf4j
@Component
public class SecEdgarClient {

    private static final String USER_AGENT = "MeTradingPlat contrerasdaniel142@gmail.com";
    private static final String TICKERS_URL = "https://www.sec.gov/files/company_tickers.json";
    private static final String FACTS_URL = "https://data.sec.gov/api/xbrl/companyfacts/CIK%010d.json";
    private static final long MIN_REQUEST_INTERVAL_MS = 110;

    private final HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();
    private final ObjectMapper objectMapper = new ObjectMapper();
    private Map<String, Integer> tickerToCik;

    public Map<String, Long> fetchSharesOutstanding(List<String> symbols) {
        Map<String, Integer> ciks = tickerToCikMap();
        Map<String, Long> result = new HashMap<>();
        long lastRequest = 0;

        for (String symbol : symbols) {
            Integer cik = ciks.get(symbol.toUpperCase());
            if (cik == null) continue;

            long wait = MIN_REQUEST_INTERVAL_MS - (System.currentTimeMillis() - lastRequest);
            if (wait > 0) {
                try { Thread.sleep(wait); } catch (InterruptedException e) { Thread.currentThread().interrupt(); break; }
            }
            lastRequest = System.currentTimeMillis();

            Long shares = fetchSharesForCik(cik);
            if (shares != null) result.put(symbol.toUpperCase(), shares);
        }
        log.info("SEC EDGAR: fetched sharesOutstanding for {}/{} symbols", result.size(), symbols.size());
        return result;
    }

    private synchronized Map<String, Integer> tickerToCikMap() {
        if (tickerToCik != null) return tickerToCik;
        try {
            Map<String, Integer> map = new HashMap<>();
            JsonNode root = get(TICKERS_URL);
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

    private Long fetchSharesForCik(int cik) {
        try {
            JsonNode shares = get(String.format(FACTS_URL, cik))
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

    private JsonNode get(String url) throws Exception {
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .header("User-Agent", USER_AGENT)
                .timeout(Duration.ofSeconds(15))
                .GET()
                .build();
        HttpResponse<InputStream> response = httpClient.send(request, HttpResponse.BodyHandlers.ofInputStream());
        return objectMapper.readTree(response.body());
    }
}
