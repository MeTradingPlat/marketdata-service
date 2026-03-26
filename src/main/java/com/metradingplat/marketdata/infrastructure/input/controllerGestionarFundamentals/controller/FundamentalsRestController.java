package com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.controller;

import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarFundamentalsCUIntPort;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.DTOAnswer.FundamentalDataDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.mapper.FundamentalsMapper;

import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/marketdata/fundamentals")
@RequiredArgsConstructor
public class FundamentalsRestController {

    private final GestionarFundamentalsCUIntPort objGestionarFundamentalsCU;
    private final FundamentalsMapper objMapper;

    @GetMapping("/{symbol}")
    public ResponseEntity<FundamentalDataDTORespuesta> obtenerFundamentals(@PathVariable String symbol) {
        return ResponseEntity.ok(objMapper.toDTORespuesta(objGestionarFundamentalsCU.obtenerFundamentals(symbol)));
    }

    @PostMapping("/batch")
    public ResponseEntity<Map<String, FundamentalDataDTORespuesta>> obtenerFundamentalsBatch(@RequestBody List<String> symbols) {
        return ResponseEntity.ok(objGestionarFundamentalsCU.obtenerFundamentalsBatch(symbols).entrySet().stream()
                .collect(Collectors.toMap(Map.Entry::getKey, e -> objMapper.toDTORespuesta(e.getValue()))));
    }
}
