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
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import com.metradingplat.marketdata.application.input.GestionarQuoteCUIntPort;
import com.metradingplat.marketdata.domain.models.Quote;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.DTOAnswer.QuoteDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.controller.QuoteRestController;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.mapper.QuoteMapper;
import com.metradingplat.marketdata.infrastructure.input.filter.GatewayHeaderFilter;

@WebMvcTest(
    controllers = QuoteRestController.class,
    excludeFilters = @ComponentScan.Filter(type = FilterType.ASSIGNABLE_TYPE, classes = GatewayHeaderFilter.class)
)
class QuoteRestControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private GestionarQuoteCUIntPort useCase;

    @MockitoBean
    private QuoteMapper mapper;

    @BeforeEach
    void setUpMapper() {
        when(mapper.deDominioARespuesta(any(Quote.class))).thenAnswer(inv -> {
            Quote q = inv.getArgument(0);
            return QuoteDTORespuesta.builder()
                    .symbol(q.getSymbol()).bid(q.getBid()).ask(q.getAsk())
                    .last(q.getLast()).open(q.getOpen()).high(q.getHigh())
                    .low(q.getLow()).close(q.getClose()).prevClose(q.getPrevClose())
                    .volume(q.getVolume()).tradingHalted(q.getTradingHalted())
                    .tradingHaltedReason(q.getTradingHaltedReason()).beta(q.getBeta())
                    .build();
        });
    }

    @Test
    void obtenerQuote_returnsOk() throws Exception {
        Quote quote = Quote.builder()
                .symbol("AAPL")
                .bid(150.0).ask(151.0).last(150.5)
                .open(149.0).high(152.0).low(148.0)
                .close(150.0).prevClose(149.0)
                .volume(1000000.0)
                .tradingHalted(false)
                .build();

        when(useCase.obtenerQuote("AAPL")).thenReturn(quote);

        mockMvc.perform(get("/marketdata/quote/AAPL"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.symbol").value("AAPL"))
                .andExpect(jsonPath("$.bid").value(150.0))
                .andExpect(jsonPath("$.ask").value(151.0))
                .andExpect(jsonPath("$.tradingHalted").value(false));
    }
}
