package com.metradingplat.marketdata.application.input;

import com.metradingplat.marketdata.domain.models.OrderRequest;

public interface GestionarOrdersCUIntPort {
    void placeBracketOrder(OrderRequest request);
}