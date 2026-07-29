package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.models.ActiveEquity;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * Universo de equities de EEUU tradeables, usado tanto por el preload REST
 * de fundamentales como por FundamentalsConnectionPool -- una sola llamada
 * REST cara (getAllActiveEquities, paginada) en vez de una por consumidor.
 */
@Component
@RequiredArgsConstructor
public class EquitiesUniverseProvider {

    private static final List<String> ALL_US_MARKETS = List.of("XNAS", "XNYS", "XASE", "ARCX", "BATS", "OTC");

    private final TastyTradeClient tastyTradeClient;

    public List<String> getUniverse() {
        List<ActiveEquity> allEquities = tastyTradeClient.getAllActiveEquities();
        Set<String> targetMarkets = new HashSet<>(ALL_US_MARKETS);
        return allEquities.stream()
                .filter(eq -> eq.getListedMarket() != null && targetMarkets.contains(eq.getListedMarket().toUpperCase()))
                .map(ActiveEquity::getSymbol)
                .distinct()
                .toList();
    }
}
