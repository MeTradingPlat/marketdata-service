package com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.controller;

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

import com.metradingplat.marketdata.application.input.GestionarEarningsCUIntPort;
import com.metradingplat.marketdata.domain.models.EarningsReport;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.DTOAnswer.EarningsReportDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.mapper.EarningsMapper;

import jakarta.validation.constraints.NotNull;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/marketdata")
@RequiredArgsConstructor
@Validated
public class EarningsRestController {

    private final GestionarEarningsCUIntPort objGestionarEarningsCUInt;
    private final EarningsMapper objMapper;

    @GetMapping("/earnings/{symbol}")
    public ResponseEntity<EarningsReportDTORespuesta> obtenerEarnings(
            @PathVariable("symbol") @NotNull String symbol) {
        EarningsReport earnings = this.objGestionarEarningsCUInt.obtenerProximoEarnings(symbol);
        EarningsReportDTORespuesta respuesta = this.objMapper.deDominioARespuesta(earnings);
        return ResponseEntity.ok(respuesta);
    }

    @PostMapping("/earnings/batch")
    public ResponseEntity<Map<String, EarningsReportDTORespuesta>> obtenerEarningsBatch(
            @RequestBody List<String> symbols) {
        return ResponseEntity.ok(this.objGestionarEarningsCUInt.obtenerEarningsBatch(symbols).entrySet().stream()
                .collect(Collectors.toMap(
                        Map.Entry::getKey,
                        e -> this.objMapper.deDominioARespuesta(e.getValue()))));
    }
}
