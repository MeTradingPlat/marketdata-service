package com.metradingplat.marketdata.domain.usecases;

import com.metradingplat.marketdata.application.input.GestionarOptionsCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.OptionChain;
import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarOptionsCUAdapter implements GestionarOptionsCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    @Override
    public OptionChain obtenerOptionChain(String symbol) {
        return objExternalGateway.getOptionChain(symbol);
    }
}
