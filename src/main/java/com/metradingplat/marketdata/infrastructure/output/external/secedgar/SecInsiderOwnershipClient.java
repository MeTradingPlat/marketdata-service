package com.metradingplat.marketdata.infrastructure.output.external.secedgar;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;
import java.time.Duration;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.TreeSet;
import java.util.function.BiConsumer;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

import org.springframework.stereotype.Component;

import lombok.extern.slf4j.Slf4j;

/**
 * Insiders (Form 3/4/5) reportan sus tenencias en un ZIP trimestral bulk de
 * la SEC -- cero requests por simbolo, igual que companyfacts.zip. Se usa
 * para restarle a sharesOutstanding la porcion que tienen bloqueada los
 * insiders y asi acercar floatShares al valor real (ver SecBeneficialOwnersClient
 * para la otra mitad: holders institucionales de 5%+ via Schedule 13D/13G).
 */
@Slf4j
@Component
public class SecInsiderOwnershipClient {

    private static final String USER_AGENT = SecTickerCikLookup.USER_AGENT;
    private static final String ZIP_URL_TEMPLATE =
            "https://www.sec.gov/files/structureddata/data/insider-transactions-data-sets/%dq%d_form345.zip";
    private static final Path CACHE_DIR = Path.of("/app/secedgar-cache");

    // Un insider que no opero en el trimestre mas reciente simplemente no
    // aparece en ese ZIP -- su tenencia real sigue siendo la de su ultimo
    // Form 4, que puede ser de trimestres anteriores. Hay que mirar hacia
    // atras varios trimestres y quedarse con el mas reciente POR owner,
    // no solo el ultimo trimestre publicado (confirmado con datos reales:
    // de 14 directores de una empresa de prueba, solo 2 habian operado en
    // el trimestre mas reciente).
    private static final int LOOKBACK_QUARTERS = 8;

    private final HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();

    private record AccessionInfo(String symbol, LocalDate filingDate) { }

    // ownerCiks se expone para que SecBeneficialOwnersClient pueda excluir
    // a un holder 5%+ que tambien sea insider (ej. un founder-CEO) -- sin
    // esto, esa posicion se restaria dos veces de sharesOutstanding.
    public record InsiderOwnership(long shares, Set<String> ownerCiks) { }

    public Map<String, InsiderOwnership> fetchInsiderShares(List<String> symbols) {
        Set<String> targets = new HashSet<>();
        for (String symbol : symbols) targets.add(symbol.toUpperCase());
        if (targets.isEmpty()) return Map.of();

        try {
            List<Path> zipFiles = ensureCachedZips(LOOKBACK_QUARTERS);
            return parseInsiderHoldings(zipFiles, targets);
        } catch (Exception e) {
            log.warn("SEC insider ownership bulk processing failed: {}", e.getMessage());
            return Map.of();
        }
    }

    private Map<String, InsiderOwnership> parseInsiderHoldings(List<Path> zipFiles, Set<String> targets) throws Exception {
        Map<String, AccessionInfo> accessions = new HashMap<>();
        for (Path zipFile : zipFiles) {
            forEachRow(zipFile, "SUBMISSION.tsv", (headers, cols) -> {
                String symbol = value(headers, cols, "ISSUERTRADINGSYMBOL");
                if (symbol == null || symbol.isBlank() || !targets.contains(symbol.toUpperCase())) return;
                String accession = value(headers, cols, "ACCESSION_NUMBER");
                accessions.put(accession, new AccessionInfo(symbol.toUpperCase(), parseDate(value(headers, cols, "FILING_DATE"))));
            });
        }
        if (accessions.isEmpty()) return Map.of();

        Map<String, TreeSet<String>> ownerGroups = new HashMap<>();
        Map<String, Long> holdingsByAccession = new HashMap<>();
        for (Path zipFile : zipFiles) {
            forEachRow(zipFile, "REPORTINGOWNER.tsv", (headers, cols) -> {
                String accession = value(headers, cols, "ACCESSION_NUMBER");
                if (!accessions.containsKey(accession)) return;
                ownerGroups.computeIfAbsent(accession, k -> new TreeSet<>()).add(value(headers, cols, "RPTOWNERCIK"));
            });
            forEachRow(zipFile, "NONDERIV_HOLDING.tsv", (headers, cols) -> {
                String accession = value(headers, cols, "ACCESSION_NUMBER");
                if (!accessions.containsKey(accession)) return;
                long shares = parseLong(value(headers, cols, "SHRS_OWND_FOLWNG_TRANS"));
                holdingsByAccession.merge(accession, shares, Long::sum);
            });
        }

        return aggregateLatestPerOwnerGroup(accessions, ownerGroups, holdingsByAccession);
    }

    private record LatestHolding(String symbol, LocalDate filingDate, long shares, TreeSet<String> owners) { }

    private Map<String, InsiderOwnership> aggregateLatestPerOwnerGroup(
            Map<String, AccessionInfo> accessions, Map<String, TreeSet<String>> ownerGroups,
            Map<String, Long> holdingsByAccession) {
        Map<String, LatestHolding> latestByGroup = new HashMap<>();
        for (var entry : accessions.entrySet()) {
            TreeSet<String> owners = ownerGroups.get(entry.getKey());
            if (owners == null || owners.isEmpty()) continue;
            String symbol = entry.getValue().symbol();
            String groupKey = symbol + "|" + String.join(",", owners);
            long shares = holdingsByAccession.getOrDefault(entry.getKey(), 0L);
            LatestHolding current = latestByGroup.get(groupKey);
            if (current == null || entry.getValue().filingDate().isAfter(current.filingDate())) {
                latestByGroup.put(groupKey, new LatestHolding(symbol, entry.getValue().filingDate(), shares, owners));
            }
        }

        Map<String, Long> sharesBySymbol = new HashMap<>();
        Map<String, Set<String>> ciksBySymbol = new HashMap<>();
        for (LatestHolding holding : latestByGroup.values()) {
            sharesBySymbol.merge(holding.symbol(), holding.shares(), Long::sum);
            ciksBySymbol.computeIfAbsent(holding.symbol(), k -> new HashSet<>()).addAll(holding.owners());
        }

        Map<String, InsiderOwnership> result = new HashMap<>();
        for (String symbol : sharesBySymbol.keySet()) {
            result.put(symbol, new InsiderOwnership(sharesBySymbol.get(symbol), ciksBySymbol.get(symbol)));
        }
        log.info("SEC insider ownership: aggregated holdings for {} symbols", result.size());
        return result;
    }

    private void forEachRow(Path zipFile, String entryName, BiConsumer<String[], String[]> rowHandler) throws Exception {
        try (ZipInputStream zip = new ZipInputStream(Files.newInputStream(zipFile))) {
            ZipEntry entry;
            while ((entry = zip.getNextEntry()) != null) {
                if (!entry.getName().equals(entryName)) continue;
                BufferedReader reader = new BufferedReader(new InputStreamReader(zip, StandardCharsets.UTF_8));
                String headerLine = reader.readLine();
                if (headerLine == null) return;
                String[] headers = headerLine.split("\t", -1);
                String line;
                while ((line = reader.readLine()) != null) {
                    rowHandler.accept(headers, line.split("\t", -1));
                }
                return;
            }
            throw new IllegalStateException(entryName + " not found in " + zipFile);
        }
    }

    private String value(String[] headers, String[] cols, String columnName) {
        for (int i = 0; i < headers.length; i++) {
            if (headers[i].equals(columnName)) return i < cols.length ? cols[i] : null;
        }
        return null;
    }

    private LocalDate parseDate(String value) {
        try { return value == null || value.isBlank() ? LocalDate.MIN : LocalDate.parse(value); }
        catch (Exception e) { return LocalDate.MIN; }
    }

    private long parseLong(String value) {
        if (value == null || value.isBlank()) return 0L;
        try { return (long) Double.parseDouble(value); }
        catch (NumberFormatException e) { return 0L; }
    }

    private List<Path> ensureCachedZips(int lookbackQuarters) throws Exception {
        Files.createDirectories(CACHE_DIR);
        List<Path> zipFiles = new ArrayList<>();
        for (int[] quarter : recentQuarters(lookbackQuarters)) {
            Path target = CACHE_DIR.resolve("insider_" + quarter[0] + "q" + quarter[1] + ".zip");
            if (Files.exists(target) && Files.size(target) > 0) {
                zipFiles.add(target);
                continue;
            }
            if (tryDownload(String.format(ZIP_URL_TEMPLATE, quarter[0], quarter[1]), target)) {
                zipFiles.add(target);
            }
            // Si un trimestre todavia no se publico (el mas reciente, por
            // ejemplo) simplemente se omite -- no es un error, los demas
            // trimestres ya cubren el lookback.
        }
        if (zipFiles.isEmpty()) {
            throw new IllegalStateException("No insider ownership ZIP available for the last " + lookbackQuarters + " quarters");
        }
        cleanupOldCacheFiles(zipFiles);
        return zipFiles;
    }

    private void cleanupOldCacheFiles(List<Path> keep) {
        try (var files = Files.list(CACHE_DIR)) {
            files.filter(p -> p.getFileName().toString().startsWith("insider_") && !keep.contains(p))
                    .forEach(p -> { try { Files.deleteIfExists(p); } catch (Exception ignored) { } });
        } catch (Exception ignored) { }
    }

    private boolean tryDownload(String url, Path target) {
        Path tmp = target.resolveSibling(target.getFileName() + ".tmp");
        try {
            log.info("SEC insider ownership: downloading {}", url);
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create(url))
                    .header("User-Agent", USER_AGENT)
                    .timeout(Duration.ofMinutes(10))
                    .GET()
                    .build();
            HttpResponse<Path> response = httpClient.send(request, HttpResponse.BodyHandlers.ofFile(tmp));
            if (response.statusCode() != 200) {
                Files.deleteIfExists(tmp);
                return false;
            }
            Files.move(tmp, target, StandardCopyOption.REPLACE_EXISTING);
            return true;
        } catch (Exception e) {
            log.debug("SEC insider ownership ZIP not available at {}: {}", url, e.getMessage());
            try { Files.deleteIfExists(tmp); } catch (Exception ignored) { }
            return false;
        }
    }

    private List<int[]> recentQuarters(int count) {
        List<int[]> quarters = new ArrayList<>();
        LocalDate now = LocalDate.now();
        int year = now.getYear();
        int quarter = (now.getMonthValue() - 1) / 3 + 1;
        for (int i = 0; i < count; i++) {
            quarters.add(new int[]{year, quarter});
            quarter--;
            if (quarter == 0) { quarter = 4; year--; }
        }
        return quarters;
    }
}
