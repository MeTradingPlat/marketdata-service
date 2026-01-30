package com.metradingplat.marketdata.application.input;

import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;

public interface GestionarOrdersCUIntPort {
    void placeBracketOrder(OrderRequest request);

    OrderResponse placeBracketOrderWithResponse(BracketOrder order);

    void cancelOrder(String orderId);
}