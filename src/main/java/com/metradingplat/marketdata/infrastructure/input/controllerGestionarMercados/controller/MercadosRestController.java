package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.controller;

import java.util.List;

import org.springframework.http.ResponseEntity;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarMercadosCUIntPort;
import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.DTOAnswer.ActiveEquityDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.DTOAnswer.MercadoDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.mapper.MercadosMapper;

import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/marketdata")
@RequiredArgsConstructor
@Validated
public class MercadosRestController {

    private final GestionarMercadosCUIntPort objGestionarMercadosCUInt;
    private final MercadosMapper objMapper;

    @GetMapping("/markets")
    public ResponseEntity<List<MercadoDTORespuesta>> listarMercados() {
        List<EnumMercado> mercados = this.objGestionarMercadosCUInt.listarMercados();
        List<MercadoDTORespuesta> respuesta = this.objMapper.deMercadosARespuestas(mercados);
        return ResponseEntity.ok(respuesta);
    }

    @GetMapping("/symbols")
    public ResponseEntity<List<ActiveEquityDTORespuesta>> obtenerSimbolos(
            @RequestParam("markets") List<String> markets) {
        List<ActiveEquity> activeEquities = this.objGestionarMercadosCUInt.obtenerSimbolosPorMercados(markets);
        List<ActiveEquityDTORespuesta> respuesta = this.objMapper.deActiveEquitiesARespuestas(activeEquities);
        return ResponseEntity.ok(respuesta);
    }
}
