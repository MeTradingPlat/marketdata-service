package com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.controller;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.domain.models.VwapQuote;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Slf4j
@RestController
@RequestMapping("/marketdata")
@RequiredArgsConstructor
public class StreamingController {

    private final TastyTradeService tastyTradeService;

    @PostMapping("/subscribe")
    public ResponseEntity<Map<String, String>> subscribeBatch(@RequestBody List<String> symbols) {
        log.info("Subscribe request: {} symbols", symbols.size());
        tastyTradeService.subscribeBatch(symbols);
        return ResponseEntity.ok(Map.of(
            "status", "subscribed",
            "count", String.valueOf(symbols.size())
        ));
    }

    @PostMapping("/unsubscribe")
    public ResponseEntity<Map<String, String>> unsubscribeBatch(@RequestBody List<String> symbols) {
        return ResponseEntity.status(403).body(Map.of(
            "status", "blocked",
            "message", "Fundamentals auto-subscribe is permanent. Unsubscribe not allowed."
        ));
    }

    @GetMapping("/subscriptions/count")
    public ResponseEntity<Map<String, Object>> subscriptionCount() {
        return ResponseEntity.ok(Map.of(
            "activeSubscriptions", tastyTradeService.getActiveSubscriptionCount()
        ));
    }

    @PostMapping("/quotes/cached")
    public ResponseEntity<Map<String, Double>> getCachedQuotes(@RequestBody List<String> symbols) {
        Map<String, Double> prices = tastyTradeService.getCachedPrices(symbols);
        log.debug("Cached quotes: {}/{} symbols found", prices.size(), symbols.size());
        return ResponseEntity.ok(prices);
    }

    @PostMapping("/vwap/rest")
    public ResponseEntity<Map<String, VwapQuote>> getVwapQuotes(@RequestBody List<String> symbols) {
        Map<String, VwapQuote> quotes = tastyTradeService.getVwapQuotes(symbols);
        log.info("VWAP quotes: {}/{} symbols", quotes.size(), symbols.size());
        return ResponseEntity.ok(quotes);
    }

    @PostMapping("/fundamentals/realtime")
    public ResponseEntity<Map<String, FundamentalData>> getRealtimeFundamentals(@RequestBody List<String> symbols) {
        Map<String, FundamentalData> cached = tastyTradeService.getCachedFundamentals(symbols);
        List<String> missing = new ArrayList<>();

        for (String sym : symbols) {
            if (!cached.containsKey(sym.toUpperCase())) {
                missing.add(sym);
            }
        }

        if (!missing.isEmpty()) {
            log.info("Cache miss for {}/{} symbols, loading now", missing.size(), symbols.size());
            Map<String, FundamentalData> loaded = tastyTradeService.getFundamentalsBatch(missing);
            for (var entry : loaded.entrySet()) {
                cached.put(entry.getKey(), entry.getValue());
            }
        }

        for (String sym : symbols) {
            cached.computeIfAbsent(sym.toUpperCase(),
                    k -> FundamentalData.builder().symbol(k).build());
        }

        log.info("Realtime fundamentals: {}/{} from cache ({} loading async)",
                cached.size(), symbols.size(), missing.size());
        return ResponseEntity.ok(cached);
    }
}
