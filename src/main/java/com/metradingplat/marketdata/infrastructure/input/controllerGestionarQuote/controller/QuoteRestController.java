package com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.controller;

import org.springframework.http.ResponseEntity;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarQuoteCUIntPort;
import com.metradingplat.marketdata.domain.models.Quote;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.DTOAnswer.QuoteDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.mapper.QuoteMapper;

import jakarta.validation.constraints.NotNull;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/marketdata")
@RequiredArgsConstructor
@Validated
public class QuoteRestController {

    private final GestionarQuoteCUIntPort objGestionarQuoteCUInt;
    private final QuoteMapper objMapper;

    @GetMapping("/quote/{symbol}")
    public ResponseEntity<QuoteDTORespuesta> obtenerQuote(
            @PathVariable("symbol") @NotNull String symbol) {
        Quote quote = this.objGestionarQuoteCUInt.obtenerQuote(symbol);
        QuoteDTORespuesta respuesta = this.objMapper.deDominioARespuesta(quote);
        return ResponseEntity.ok(respuesta);
    }
}
