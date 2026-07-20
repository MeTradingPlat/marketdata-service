package com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.controller;

import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

import org.springframework.http.ResponseEntity;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarQuoteCUIntPort;
import com.metradingplat.marketdata.domain.models.Quote;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.DTOAnswer.QuoteDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.mapper.QuoteMapper;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeClient;

import jakarta.validation.constraints.NotNull;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Slf4j
@RestController
@RequestMapping("/marketdata")
@RequiredArgsConstructor
@Validated
public class QuoteRestController {

    private final GestionarQuoteCUIntPort objGestionarQuoteCUInt;
    private final QuoteMapper objMapper;
    private final TastyTradeClient tastyTradeClient;

    @GetMapping("/quote/{symbol}")
    public ResponseEntity<QuoteDTORespuesta> obtenerQuote(
            @PathVariable("symbol") @NotNull String symbol) {
        Quote quote = this.objGestionarQuoteCUInt.obtenerQuote(symbol);
        QuoteDTORespuesta respuesta = this.objMapper.deDominioARespuesta(quote);
        return ResponseEntity.ok(respuesta);
    }

    @PostMapping("/quote/batch")
    public ResponseEntity<List<QuoteDTORespuesta>> obtenerQuotesBatch(@RequestBody List<String> symbols) {
        log.info("Batch quotes: {} symbols via TastyTrade REST", symbols.size());
        List<Map<String, Object>> rawItems = tastyTradeClient.getMarketDataBatch(symbols);
        List<QuoteDTORespuesta> quotes = rawItems.stream()
                .map(item -> new QuoteDTORespuesta(
                        (String) item.get("symbol"),
                        toDouble(item.get("bid")),
                        toDouble(item.get("ask")),
                        toDouble(item.get("last")),
                        toDouble(item.get("open")),
                        toDouble(item.get("day-high-price")),
                        toDouble(item.get("day-low-price")),
                        toDouble(item.get("close")),
                        toDouble(item.get("prev-close")),
                        toDouble(item.get("volume")),
                        item.get("is-trading-halted") != null ? (Boolean) item.get("is-trading-halted") : false,
                        item.get("halt-reason") != null ? item.get("halt-reason").toString() : null,
                        toDouble(item.get("beta"))
                ))
                .collect(Collectors.toList());
        log.info("Batch quotes: returned {}/{} quotes", quotes.size(), symbols.size());
        return ResponseEntity.ok(quotes);
    }

    private Double toDouble(Object value) {
        if (value instanceof Number n) return n.doubleValue();
        return null;
    }

    private Boolean toBoolean(Object value) {
        if (value instanceof Boolean b) return b;
        return null;
    }
}
