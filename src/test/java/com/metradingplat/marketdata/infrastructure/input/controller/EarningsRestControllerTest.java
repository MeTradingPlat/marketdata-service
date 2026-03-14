package com.metradingplat.marketdata.infrastructure.input.controller;

import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

import java.time.LocalDate;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.ComponentScan;
import org.springframework.context.annotation.FilterType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import com.metradingplat.marketdata.application.input.GestionarEarningsCUIntPort;
import com.metradingplat.marketdata.domain.models.EarningsReport;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.DTOAnswer.EarningsReportDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.controller.EarningsRestController;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.mapper.EarningsMapper;
import com.metradingplat.marketdata.infrastructure.input.filter.GatewayHeaderFilter;

@WebMvcTest(
    controllers = EarningsRestController.class,
    excludeFilters = @ComponentScan.Filter(type = FilterType.ASSIGNABLE_TYPE, classes = GatewayHeaderFilter.class)
)
class EarningsRestControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private GestionarEarningsCUIntPort useCase;

    @MockitoBean
    private EarningsMapper mapper;

    @BeforeEach
    void setUpMapper() {
        when(mapper.deDominioARespuesta(any(EarningsReport.class))).thenAnswer(inv -> {
            EarningsReport r = inv.getArgument(0);
            return EarningsReportDTORespuesta.builder()
                    .symbol(r.getSymbol()).occurredDate(r.getOccurredDate())
                    .eps(r.getEps()).daysUntilEarnings(r.getDaysUntilEarnings())
                    .build();
        });
    }

    @Test
    void obtenerEarnings_returnsOk() throws Exception {
        EarningsReport report = EarningsReport.builder()
                .symbol("AAPL")
                .occurredDate(LocalDate.of(2025, 1, 15))
                .eps(2.5)
                .daysUntilEarnings(45L)
                .build();

        when(useCase.obtenerProximoEarnings("AAPL")).thenReturn(report);

        mockMvc.perform(get("/api/marketdata/earnings/AAPL"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.symbol").value("AAPL"))
                .andExpect(jsonPath("$.eps").value(2.5))
                .andExpect(jsonPath("$.daysUntilEarnings").value(45));
    }
}
