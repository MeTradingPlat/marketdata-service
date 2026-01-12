package com.metradingplat.marketdata.domain.usecases;

import com.metradingplat.marketdata.application.input.GestionarRealTimeCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarRealTimeCUAdapter implements GestionarRealTimeCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objGestionarComunicacionExterna;

    @Override
    public void subscribeToSymbol(String symbol) {
        this.objGestionarComunicacionExterna.subscribe(symbol);
    }

    @Override
    public void unsubscribeFromSymbol(String symbol) {
        this.objGestionarComunicacionExterna.unsubscribe(symbol);
    }
}
