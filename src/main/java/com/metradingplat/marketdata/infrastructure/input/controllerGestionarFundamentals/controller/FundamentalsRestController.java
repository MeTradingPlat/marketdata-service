package com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.controller;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.DTOAnswer.FundamentalDataDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.mapper.FundamentalsMapper;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Slf4j
@RestController
@RequestMapping("/marketdata/fundamentals")
@RequiredArgsConstructor
public class FundamentalsRestController {

    private final TastyTradeService tastyTradeService;
    private final FundamentalsMapper objMapper;

    @GetMapping("/{symbol}")
    public ResponseEntity<FundamentalDataDTORespuesta> obtenerFundamentals(@PathVariable String symbol) {
        Map<String, FundamentalData> cached = tastyTradeService.getCachedFundamentals(List.of(symbol));
        FundamentalData data = cached.getOrDefault(symbol.toUpperCase(),
                FundamentalData.builder().symbol(symbol).build());
        if (!cached.containsKey(symbol.toUpperCase())) {
            java.util.concurrent.CompletableFuture.runAsync(() ->
                    tastyTradeService.getFundamentalsBatch(List.of(symbol)));
        }
        return ResponseEntity.ok(objMapper.toDTORespuesta(data));
    }

    @PostMapping("/batch")
    public ResponseEntity<Map<String, FundamentalDataDTORespuesta>> obtenerFundamentalsBatch(@RequestBody List<String> symbols) {
        Map<String, FundamentalData> cached = tastyTradeService.getCachedFundamentals(symbols);
        List<String> missing = new ArrayList<>();
        Map<String, FundamentalDataDTORespuesta> result = new HashMap<>();

        for (String sym : symbols) {
            String upper = sym.toUpperCase();
            FundamentalData d = cached.get(upper);
            if (d != null) {
                result.put(upper, objMapper.toDTORespuesta(d));
            } else {
                missing.add(sym);
                result.put(upper, objMapper.toDTORespuesta(
                        FundamentalData.builder().symbol(upper).build()));
            }
        }

        if (!missing.isEmpty()) {
            log.info("Fundamentals batch: {} cache misses loading async", missing.size());
            java.util.concurrent.CompletableFuture.runAsync(() ->
                    tastyTradeService.getFundamentalsBatch(missing));
        }

        return ResponseEntity.ok(result);
    }
}
