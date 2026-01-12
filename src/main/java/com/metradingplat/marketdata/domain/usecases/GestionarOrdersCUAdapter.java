package com.metradingplat.marketdata.domain.usecases;

import com.metradingplat.marketdata.application.input.GestionarOrdersCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.OrderRequest;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarOrdersCUAdapter implements GestionarOrdersCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objGestionarComunicacionExterna;

    @Override
    public void placeBracketOrder(OrderRequest request) {
        this.objGestionarComunicacionExterna.sendOrder(request);
    }
}
