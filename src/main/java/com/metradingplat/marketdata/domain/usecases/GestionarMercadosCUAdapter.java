package com.metradingplat.marketdata.domain.usecases;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import com.metradingplat.marketdata.application.input.GestionarMercadosCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.PagedActiveEquities;


import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@RequiredArgsConstructor
@Slf4j
public class GestionarMercadosCUAdapter implements GestionarMercadosCUIntPort {

    private static final Duration CACHE_TTL = Duration.ofDays(1);
    private static final int PAGE_SIZE = 5000;

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    private volatile List<ActiveEquity> cachedEquities = Collections.emptyList();
    private volatile Instant lastRefresh = Instant.EPOCH;

    @Override
    public List<EnumMercado> listarMercados() {
        return Arrays.asList(EnumMercado.values());
    }

    @Override
    public List<String> obtenerMercadosDisponibles() {
        cargarEquitiesSiNecesario();
        return cachedEquities.stream()
                .map(ActiveEquity::getListedMarket)
                .filter(m -> m != null && !m.isBlank())
                .distinct()
                .sorted()
                .collect(Collectors.toList());
    }

    @Override
    public List<ActiveEquity> obtenerSimbolosPorMercados(List<String> markets) {
        cargarEquitiesSiNecesario();
        return filtrarPorMercados(markets).collect(Collectors.toList());
    }

    @Override
    public PagedActiveEquities buscarSimbolos(String query, List<String> markets, int page, int size) {
        cargarEquitiesSiNecesario();

        String needle = query != null ? query.trim().toLowerCase() : "";
        List<ActiveEquity> matched = filtrarPorMercados(markets)
                .filter(eq -> needle.isEmpty()
                        || (eq.getSymbol() != null && eq.getSymbol().toLowerCase().contains(needle))
                        || (eq.getDescription() != null && eq.getDescription().toLowerCase().contains(needle)))
                // Sin esto el orden es el de llegada de TastyTrade (arbitrario) --
                // buscar "aapl" mostraba a AAPL en la mitad de la lista en vez
                // de primero. Ahora prioriza: simbolo exacto > simbolo empieza
                // con lo escrito > simbolo lo contiene > solo la descripcion
                // coincide, alfabetico como desempate dentro de cada nivel.
                .sorted(java.util.Comparator
                        .<ActiveEquity>comparingInt(eq -> relevanceRank(eq, needle))
                        .thenComparing(eq -> eq.getSymbol(), String.CASE_INSENSITIVE_ORDER))
                .collect(Collectors.toList());

        List<ActiveEquity> pageItems = matched.stream()
                .skip((long) page * size)
                .limit(size)
                .collect(Collectors.toList());

        return new PagedActiveEquities(pageItems, matched.size());
    }

    private static int relevanceRank(ActiveEquity eq, String needle) {
        if (needle.isEmpty()) return 0;
        String symbol = eq.getSymbol() != null ? eq.getSymbol().toLowerCase() : "";
        if (symbol.equals(needle)) return 0;
        if (symbol.startsWith(needle)) return 1;
        if (symbol.contains(needle)) return 2;
        return 3; // solo coincide la descripcion
    }

    private Stream<ActiveEquity> filtrarPorMercados(List<String> markets) {
        Set<String> userMarkets = markets != null && !markets.isEmpty()
                ? markets.stream().map(String::toUpperCase).collect(Collectors.toSet())
                : Set.of();

        if (userMarkets.isEmpty()) {
            return cachedEquities.stream();
        }

        // If market exists in EnumMercado, use its MIC codes.
        // Otherwise, assume the user provided the MIC code directly (dynamic case).
        Set<String> initialMicFilter = EnumMercado.toMicCodes(userMarkets);
        final Set<String> micFilter = initialMicFilter.isEmpty() ? userMarkets : initialMicFilter;

        log.info("Filtrando equities por mercados: {} -> MIC codes: {}, total en cache: {}", userMarkets, micFilter, cachedEquities.size());

        return cachedEquities.stream()
                .filter(eq -> eq.getListedMarket() != null
                        && micFilter.contains(eq.getListedMarket().toUpperCase()));
    }

    private synchronized void cargarEquitiesSiNecesario() {
        if (!cachedEquities.isEmpty() && Instant.now().isBefore(lastRefresh.plus(CACHE_TTL))) {
            return;
        }

        log.info("Cargando cache de equities activos desde TastyTrade...");
        List<ActiveEquity> allEquities = new ArrayList<>();
        int page = 0;

        while (true) {
            List<ActiveEquity> batch = this.objExternalGateway.getActiveEquities(page, PAGE_SIZE);
            if (batch.isEmpty()) break;
            allEquities.addAll(batch);
            if (batch.size() < PAGE_SIZE) break;
            page++;
            
            try {
                Thread.sleep(500); // Throttling preventivo base (Steering Rule)
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            }
        }

        cachedEquities = Collections.unmodifiableList(allEquities);
        lastRefresh = Instant.now();
        log.info("Cache de equities cargado: {} simbolos", cachedEquities.size());

        // Debug: mostrar valores distintos de listed-market
        var distinctMarkets = allEquities.stream()
                .map(ActiveEquity::getListedMarket)
                .filter(m -> m != null)
                .collect(Collectors.toSet());
        log.info("Valores distintos de listed-market: {}", distinctMarkets);
    }
}
