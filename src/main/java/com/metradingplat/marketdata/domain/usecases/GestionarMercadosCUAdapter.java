package com.metradingplat.marketdata.domain.usecases;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

import com.metradingplat.marketdata.application.input.GestionarMercadosCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;


import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@RequiredArgsConstructor
@Slf4j
public class GestionarMercadosCUAdapter implements GestionarMercadosCUIntPort {

    private static final Duration CACHE_TTL = Duration.ofHours(1);
    private static final int PAGE_SIZE = 1000;

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    private volatile List<ActiveEquity> cachedEquities = Collections.emptyList();
    private volatile Instant lastRefresh = Instant.EPOCH;

    @Override
    public List<EnumMercado> listarMercados() {
        return Arrays.asList(EnumMercado.values());
    }

    @Override
    public List<ActiveEquity> obtenerSimbolosPorMercados(List<String> markets) {
        cargarEquitiesSiNecesario();

        Set<String> userMarkets = markets != null && !markets.isEmpty()
                ? markets.stream().map(String::toUpperCase).collect(Collectors.toSet())
                : Set.of();

        if (userMarkets.isEmpty()) {
            return new ArrayList<>(cachedEquities);
        }

        // Convert user-facing codes (NYSE, NASDAQ) to TastyTrade MIC codes (XNYS, XNAS)
        Set<String> micFilter = EnumMercado.toMicCodes(userMarkets);

        log.info("Filtrando equities por mercados: {} -> MIC codes: {}, total en cache: {}", userMarkets, micFilter, cachedEquities.size());

        return cachedEquities.stream()
                .filter(eq -> eq.getListedMarket() != null
                        && micFilter.contains(eq.getListedMarket().toUpperCase()))
                .collect(Collectors.toList());
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
