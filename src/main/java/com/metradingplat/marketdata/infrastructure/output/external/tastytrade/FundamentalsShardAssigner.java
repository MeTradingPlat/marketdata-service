package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

/**
 * Reparte el universo de simbolos en hasta connectionCount shards de maximo
 * symbolsPerConnection cada uno -- nunca descarta simbolos en silencio si el
 * pool configurado no alcanza a cubrir todo el universo, avisa por log.
 */
@Slf4j
@Component
public class FundamentalsShardAssigner {

    public List<List<String>> assign(List<String> universe, int connectionCount, int symbolsPerConnection) {
        int capacity = connectionCount * symbolsPerConnection;
        if (universe.size() > capacity) {
            log.warn("Fundamentals universe ({}) exceeds pool capacity ({} connections x {} symbols = {}) -- "
                            + "{} symbols will not get live pool coverage until the pool is resized",
                    universe.size(), connectionCount, symbolsPerConnection, capacity, universe.size() - capacity);
        }
        List<List<String>> shards = new ArrayList<>();
        for (int i = 0; i < universe.size() && shards.size() < connectionCount; i += symbolsPerConnection) {
            shards.add(new ArrayList<>(universe.subList(i, Math.min(i + symbolsPerConnection, universe.size()))));
        }
        return shards;
    }
}
