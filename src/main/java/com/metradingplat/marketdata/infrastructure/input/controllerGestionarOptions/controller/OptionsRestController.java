package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.controller;

import com.metradingplat.marketdata.application.input.GestionarOptionsCUIntPort;
import com.metradingplat.marketdata.domain.models.OptionChain;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.DTOAnswer.OptionChainDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.mapper.OptionsMapper;
import lombok.RequiredArgsConstructor;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/marketdata/options")
@RequiredArgsConstructor
public class OptionsRestController {

    private final GestionarOptionsCUIntPort optionsCU;
    private final OptionsMapper mapper;

    @GetMapping("/chain/{symbol}")
    public ResponseEntity<OptionChainDTORespuesta> getOptionChain(@PathVariable String symbol) {
        OptionChain chain = optionsCU.obtenerOptionChain(symbol);
        if (chain == null) {
            return ResponseEntity.notFound().build();
        }
        return ResponseEntity.ok(mapper.toDTORespuesta(chain));
    }
}
