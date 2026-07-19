package com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.controller;

import java.util.List;
import java.util.Map;

import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.domain.models.FundamentalData;
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

    @PostMapping("/quotes/cached")
    public ResponseEntity<Map<String, Double>> getCachedQuotes(@RequestBody List<String> symbols) {
        Map<String, Double> prices = tastyTradeService.getCachedPrices(symbols);
        log.debug("Cached quotes: {}/{} symbols found", prices.size(), symbols.size());
        return ResponseEntity.ok(prices);
    }

    @PostMapping("/fundamentals/realtime")
    public ResponseEntity<Map<String, FundamentalData>> getRealtimeFundamentals(@RequestBody List<String> symbols) {
        Map<String, FundamentalData> data = tastyTradeService.getCachedFundamentals(symbols);
        log.debug("Realtime fundamentals: {}/{} symbols found", data.size(), symbols.size());
        return ResponseEntity.ok(data);
    }
}
