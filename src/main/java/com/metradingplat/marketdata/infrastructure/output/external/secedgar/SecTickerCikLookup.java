package com.metradingplat.marketdata.infrastructure.output.external.secedgar;

import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;
import java.time.Duration;
import java.time.Instant;
import java.time.LocalDate;
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
    private static final Path CACHE_DIR = Path.of("/app/secedgar-cache");

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
            Map<String, Integer> map = parseTickers(loadCachedOrDownload());
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

    // Igual que companyfacts.zip: cacheado en disco por fecha, reutilizado
    // entre reinicios del mismo dia -- si el contenedor se reinicia varias
    // veces (ej. mientras se arregla algo), no vuelve a pedirle nada a la
    // SEC hasta el dia siguiente.
    private InputStream loadCachedOrDownload() throws Exception {
        Files.createDirectories(CACHE_DIR);
        Path target = CACHE_DIR.resolve("company_tickers_" + LocalDate.now() + ".json");
        if (Files.exists(target) && Files.size(target) > 0) {
            log.info("SEC EDGAR: reusing today's cached ticker file, no download needed");
            return Files.newInputStream(target);
        }

        Path tmp = CACHE_DIR.resolve(target.getFileName() + ".tmp");
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(TICKERS_URL))
                .header("User-Agent", USER_AGENT)
                .timeout(Duration.ofSeconds(15))
                .GET()
                .build();
        HttpResponse<Path> response = httpClient.send(request, HttpResponse.BodyHandlers.ofFile(tmp));
        if (response.statusCode() != 200) {
            Files.deleteIfExists(tmp);
            throw new IllegalStateException("SEC returned HTTP " + response.statusCode());
        }
        Files.move(tmp, target, StandardCopyOption.REPLACE_EXISTING);
        cleanupOldCacheFiles(target);
        return Files.newInputStream(target);
    }

    private void cleanupOldCacheFiles(Path keep) {
        try (var files = Files.list(CACHE_DIR)) {
            files.filter(p -> p.getFileName().toString().startsWith("company_tickers_") && !p.equals(keep))
                    .forEach(p -> { try { Files.deleteIfExists(p); } catch (Exception ignored) { } });
        } catch (Exception ignored) { }
    }

    private Map<String, Integer> parseTickers(InputStream body) throws Exception {
        try (body) {
            Map<String, Integer> map = new HashMap<>();
            JsonNode root = objectMapper.readTree(body);
            for (JsonNode entry : root) {
                String ticker = entry.path("ticker").asText(null);
                int cik = entry.path("cik_str").asInt(-1);
                if (ticker != null && cik > 0) map.put(ticker.toUpperCase(), cik);
            }
            return map;
        }
    }
}
