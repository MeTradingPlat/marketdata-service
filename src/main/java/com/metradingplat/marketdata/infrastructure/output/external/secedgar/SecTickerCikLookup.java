package com.metradingplat.marketdata.infrastructure.output.external.secedgar;

import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.Instant;
import java.util.HashMap;
import java.util.Map;

import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import lombok.extern.slf4j.Slf4j;

@Slf4j
@Component
public class SecTickerCikLookup {

    static final String USER_AGENT = "MeTradingPlat contrerasdaniel142@gmail.com";
    private static final String TICKERS_URL = "https://www.sec.gov/files/company_tickers.json";

    // Este lookup se llama por-simbolo desde SecBeneficialOwnersClient -- sin
    // enfriamiento, un solo rate-limit/429 de la SEC (que responde HTML en
    // vez de JSON, forzando la excepcion) hacia que CADA simbolo del batch
    // reintentara la misma llamada fallida de inmediato, multiplicando el
    // trafico contra la SEC justo cuando ya esta limitandonos -- confirmado
    // en vivo via threaddump: 7 fallos identicos en ~200ms.
    private static final Duration FAILURE_COOLDOWN = Duration.ofMinutes(5);

    private final HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();
    private final ObjectMapper objectMapper = new ObjectMapper();
    private Map<String, Integer> tickerToCik;
    private Instant lastFailureAt;

    public synchronized Map<String, Integer> tickerToCikMap() {
        if (tickerToCik != null) return tickerToCik;
        if (lastFailureAt != null && Instant.now().isBefore(lastFailureAt.plus(FAILURE_COOLDOWN))) {
            return Map.of();
        }
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
            log.warn("SEC EDGAR: failed to load ticker->CIK mapping, backing off {} min: {}",
                    FAILURE_COOLDOWN.toMinutes(), e.getMessage());
            lastFailureAt = Instant.now();
            return Map.of();
        }
    }
}
