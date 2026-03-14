package com.metradingplat.marketdata.infrastructure.input.controller;

import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.ComponentScan;
import org.springframework.context.annotation.FilterType;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import com.metradingplat.marketdata.application.input.GestionarOrdersCUIntPort;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.OrderResponse;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.DTOAnswer.OrderResponseDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.controller.OrdersRestController;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.mapper.OrdersRestMapper;
import com.metradingplat.marketdata.infrastructure.input.filter.GatewayHeaderFilter;

@WebMvcTest(
    controllers = OrdersRestController.class,
    excludeFilters = @ComponentScan.Filter(type = FilterType.ASSIGNABLE_TYPE, classes = GatewayHeaderFilter.class)
)
class OrdersRestControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private GestionarOrdersCUIntPort useCase;

    @MockitoBean
    private OrdersRestMapper mapper;

    @BeforeEach
    void setUpMapper() {
        when(mapper.dePeticionADominio(any())).thenAnswer(inv -> {
            // Return a simple BracketOrder - the actual mapping isn't critical for controller tests
            return BracketOrder.builder().symbol("AAPL").build();
        });
        when(mapper.deDominioARespuesta(any(OrderResponse.class))).thenAnswer(inv -> {
            OrderResponse r = inv.getArgument(0);
            return OrderResponseDTORespuesta.builder()
                    .orderId(r.getOrderId()).status(r.getStatus())
                    .complexOrderId(r.getComplexOrderId())
                    .build();
        });
    }

    @Test
    void placeBracketOrder_returnsOk() throws Exception {
        OrderResponse response = OrderResponse.builder()
                .orderId("123")
                .status("RECEIVED")
                .complexOrderId("456")
                .build();

        when(useCase.placeBracketOrderWithResponse(any())).thenReturn(response);

        String json = """
                {
                    "symbol": "AAPL",
                    "action": "BUY_TO_OPEN",
                    "quantity": 10,
                    "entryPrice": 150.00,
                    "stopLossPrice": 145.00,
                    "takeProfitPrice": 160.00
                }
                """;

        mockMvc.perform(post("/api/marketdata/orders")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(json))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.orderId").value("123"))
                .andExpect(jsonPath("$.status").value("RECEIVED"));
    }

    @Test
    void placeBracketOrder_returnsBadRequest_whenMissingFields() throws Exception {
        String json = """
                {
                    "symbol": "AAPL"
                }
                """;

        mockMvc.perform(post("/api/marketdata/orders")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(json))
                .andExpect(status().isBadRequest());
    }

    @Test
    void cancelOrder_returnsNoContent() throws Exception {
        doNothing().when(useCase).cancelOrder("order-123");

        mockMvc.perform(delete("/api/marketdata/orders/order-123"))
                .andExpect(status().isNoContent());

        verify(useCase).cancelOrder("order-123");
    }
}
