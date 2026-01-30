package com.metradingplat.marketdata.domain.usecases;

import com.metradingplat.marketdata.application.input.GestionarOrdersCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarOrdersCUAdapter implements GestionarOrdersCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objGestionarComunicacionExterna;

    @Override
    public void placeBracketOrder(OrderRequest request) {
        this.objGestionarComunicacionExterna.sendOrder(request);
    }

    @Override
    public OrderResponse placeBracketOrderWithResponse(BracketOrder order) {
        return this.objGestionarComunicacionExterna.sendBracketOrder(order);
    }

    @Override
    public void cancelOrder(String orderId) {
        this.objGestionarComunicacionExterna.cancelOrder(orderId);
    }
}
