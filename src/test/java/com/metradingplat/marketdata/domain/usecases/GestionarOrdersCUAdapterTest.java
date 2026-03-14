package com.metradingplat.marketdata.domain.usecases;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

import java.math.BigDecimal;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumOrderAction;
import com.metradingplat.marketdata.domain.enums.EnumOrderType;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;

@ExtendWith(MockitoExtension.class)
class GestionarOrdersCUAdapterTest {

    @Mock
    private GestionarComunicacionExternalGatewayIntPort gateway;

    private GestionarOrdersCUAdapter adapter;

    @BeforeEach
    void setUp() {
        adapter = new GestionarOrdersCUAdapter(gateway);
    }

    @Test
    void placeBracketOrder_delegatesToGateway() {
        OrderRequest request = OrderRequest.builder()
                .symbol("AAPL")
                .action(EnumOrderAction.BUY_TO_OPEN)
                .type(EnumOrderType.LIMIT)
                .quantity(10)
                .price(new BigDecimal("150.00"))
                .build();

        adapter.placeBracketOrder(request);

        verify(gateway).sendOrder(request);
    }

    @Test
    void placeBracketOrderWithResponse_returnsGatewayResponse() {
        BracketOrder order = BracketOrder.builder()
                .symbol("AAPL")
                .action(EnumOrderAction.BUY_TO_OPEN)
                .quantity(10)
                .entryPrice(new BigDecimal("150.00"))
                .stopLossPrice(new BigDecimal("145.00"))
                .takeProfitPrice(new BigDecimal("160.00"))
                .build();

        OrderResponse expected = OrderResponse.builder()
                .orderId("123")
                .status("RECEIVED")
                .build();

        when(gateway.sendBracketOrder(order)).thenReturn(expected);

        OrderResponse result = adapter.placeBracketOrderWithResponse(order);

        assertEquals("123", result.getOrderId());
        assertEquals("RECEIVED", result.getStatus());
    }

    @Test
    void cancelOrder_delegatesToGateway() {
        adapter.cancelOrder("order-456");

        verify(gateway).cancelOrder("order-456");
    }
}
