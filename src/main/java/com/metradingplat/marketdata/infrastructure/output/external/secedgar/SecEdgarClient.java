package com.metradingplat.marketdata.infrastructure.output.external.secedgar;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;
import java.time.Duration;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Free, no-key shares-outstanding fallback for symbols TastyTrade doesn't
 * cover (OTC/penny stocks, thin small-caps). Caches SEC's nightly bulk
 * companyfacts.zip to a persistent volume once per calendar day, and
 * streams it entry-by-entry instead of buffering it whole, so peak memory
 * stays bounded by one company's JSON at a time, not the ~1.5GB archive.
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class SecEdgarClient {

    private static final String USER_AGENT = SecTickerCikLookup.USER_AGENT;
    private static final String BULK_FACTS_URL = "https://www.sec.gov/Archives/edgar/daily-index/xbrl/companyfacts.zip";
    private static final Path CACHE_DIR = Path.of("/app/secedgar-cache");

    private final SecTickerCikLookup tickerCikLookup;
    private final HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();
    private final ObjectMapper objectMapper = new ObjectMapper();

    public Map<String, Long> fetchSharesOutstanding(List<String> symbols) {
        Map<String, List<String>> cikToSymbols = new HashMap<>();
        for (String symbol : symbols) {
            Integer cik = tickerCikLookup.tickerToCikMap().get(symbol.toUpperCase());
            if (cik != null) {
                cikToSymbols.computeIfAbsent(String.format("%010d", cik), k -> new ArrayList<>()).add(symbol.toUpperCase());
            }
        }
        if (cikToSymbols.isEmpty()) return Map.of();

        // sharesOutstanding es una cifra por-entidad (la accion comun), no
        // por-ticker. Cuando varios tickers distintos comparten un CIK --
        // series de preferentes, warrants, ETNs emitidos por un banco --
        // no hay forma de saber a cual de ellos le pertenece ese numero.
        // Confirmado en vivo: 394 CIKs con 1014 tickers afectados, incluido
        // un caso donde 26 ETFs apalancados sin relacion entre si terminaban
        // con el sharesOutstanding del banco emisor de sus notas. Mejor no
        // aplicar el dato en esos casos que aplicarlo a ciegas y estar mal.
        long totalTargets = cikToSymbols.values().stream().filter(list -> list.size() == 1).count();

        Map<String, Long> result = new HashMap<>();
        try {
            Path zipFile = ensureCachedZip();
            try (ZipInputStream zip = new ZipInputStream(Files.newInputStream(zipFile))) {
                ZipEntry entry;
                while (result.size() < totalTargets && (entry = zip.getNextEntry()) != null) {
                    String cik = extractCik(entry.getName());
                    List<String> matchedSymbols = cik != null ? cikToSymbols.get(cik) : null;
                    if (matchedSymbols != null && matchedSymbols.size() == 1) {
                        Long shares = parseSharesOutstanding(zip.readAllBytes());
                        if (shares != null) {
                            result.put(matchedSymbols.get(0), shares);
                        }
                    }
                    zip.closeEntry();
                }
            }
        } catch (Exception e) {
            log.warn("SEC EDGAR bulk processing failed: {}", e.getMessage());
        }
        log.info("SEC EDGAR: fetched sharesOutstanding for {}/{} symbols", result.size(), symbols.size());
        return result;
    }

    private Path ensureCachedZip() throws Exception {
        Files.createDirectories(CACHE_DIR);
        Path target = CACHE_DIR.resolve("companyfacts_" + LocalDate.now() + ".zip");
        if (Files.exists(target) && Files.size(target) > 0) {
            log.info("SEC EDGAR: reusing today's cached bulk file, no download needed");
            return target;
        }

        log.info("SEC EDGAR: no cache for today, downloading bulk companyfacts.zip...");
        Path tmp = CACHE_DIR.resolve(target.getFileName() + ".tmp");
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(BULK_FACTS_URL))
                .header("User-Agent", USER_AGENT)
                .timeout(Duration.ofMinutes(20))
                .GET()
                .build();
        httpClient.send(request, HttpResponse.BodyHandlers.ofFile(tmp));
        Files.move(tmp, target, StandardCopyOption.REPLACE_EXISTING);
        cleanupOldCacheFiles(target);
        return target;
    }

    private void cleanupOldCacheFiles(Path keep) {
        try (var files = Files.list(CACHE_DIR)) {
            files.filter(p -> !p.equals(keep)).forEach(p -> {
                try { Files.deleteIfExists(p); } catch (Exception ignored) { }
            });
        } catch (Exception ignored) { }
    }

    private String extractCik(String entryName) {
        if (entryName == null || !entryName.startsWith("CIK") || !entryName.endsWith(".json")) return null;
        return entryName.substring(3, entryName.length() - 5);
    }

    private Long parseSharesOutstanding(byte[] entryData) {
        try {
            JsonNode facts = objectMapper.readTree(entryData).path("facts");
            Long shares = latestShareCount(facts.path("dei").path("EntityCommonStockSharesOutstanding"));
            if (shares == null) shares = latestShareCount(facts.path("us-gaap").path("CommonStockSharesOutstanding"));
            return shares;
        } catch (Exception e) {
            return null;
        }
    }

    // Piso de plausibilidad: menos de 10k acciones es practicamente siempre
    // un desglose por clase de accion (ej. la clase B del fundador) filtrado
    // por error, no el total de la empresa -- confirmado en vivo con FNKO,
    // cuyo unico dato en us-gaap:CommonStockSharesOutstanding eran 100
    // acciones de 2017 (Class B), mientras el total real es ~34M.
    private static final long MIN_PLAUSIBLE_SHARES = 10_000L;
    // Si el dato mas reciente disponible tiene mas de esto, se prefiere no
    // tener dato (null, cae al heuristico/DxLink) antes que mostrar una
    // cifra que la empresa dejo de reportar bajo ese tag hace años.
    private static final java.time.Period MAX_STALENESS = java.time.Period.ofYears(2);

    private Long latestShareCount(JsonNode concept) {
        long latestVal = 0;
        String latestFiled = "";
        String latestEnd = "";
        for (JsonNode entry : concept.path("units").path("shares")) {
            String filed = entry.path("filed").asText("");
            if (filed.compareTo(latestFiled) > 0) {
                latestFiled = filed;
                latestVal = entry.path("val").asLong(0);
                latestEnd = entry.path("end").asText("");
            }
        }
        if (latestVal < MIN_PLAUSIBLE_SHARES) return null;
        if (isStale(latestEnd)) return null;
        return latestVal;
    }

    private boolean isStale(String endDate) {
        try {
            return LocalDate.parse(endDate).isBefore(LocalDate.now().minus(MAX_STALENESS));
        } catch (Exception e) {
            return true;
        }
    }

}
