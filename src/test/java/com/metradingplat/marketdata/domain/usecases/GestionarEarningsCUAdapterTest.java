package com.metradingplat.marketdata.domain.usecases;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.EarningsReport;

@ExtendWith(MockitoExtension.class)
class GestionarEarningsCUAdapterTest {

    @Mock
    private GestionarComunicacionExternalGatewayIntPort gateway;

    private GestionarEarningsCUAdapter adapter;

    @BeforeEach
    void setUp() {
        adapter = new GestionarEarningsCUAdapter(gateway);
    }

    @Test
    void obtenerProximoEarnings_returnsMinusOne_whenNoReports() {
        when(gateway.getEarningsReports(eq("AAPL"), anyString())).thenReturn(List.of());

        EarningsReport result = adapter.obtenerProximoEarnings("AAPL");

        assertEquals("AAPL", result.getSymbol());
        assertEquals(-1L, result.getDaysUntilEarnings());
    }

    @Test
    void obtenerProximoEarnings_calculatesDaysUntilPendingQuarter() {
        LocalDate today = LocalDate.now();
        // Pending quarter ending 10 days from now => announcement = 10 + 35 = 45 days from now
        LocalDate pendingQuarterEnd = today.plusDays(10);

        // One reported quarter
        Map<String, Object> reported = new HashMap<>();
        reported.put("occurred-date", today.minusMonths(3).toString());
        reported.put("eps", 2.5);

        // One pending quarter (no eps)
        Map<String, Object> pending = new HashMap<>();
        pending.put("occurred-date", pendingQuarterEnd.toString());
        pending.put("eps", null);

        when(gateway.getEarningsReports(eq("AAPL"), anyString()))
                .thenReturn(List.of(reported, pending));

        EarningsReport result = adapter.obtenerProximoEarnings("AAPL");

        long expectedDays = ChronoUnit.DAYS.between(today, pendingQuarterEnd.plusDays(35));
        assertEquals(expectedDays, result.getDaysUntilEarnings());
        assertEquals(2.5, result.getEps());
    }

    @Test
    void obtenerProximoEarnings_estimatesNextQuarter_whenAllReported() {
        LocalDate today = LocalDate.now();
        LocalDate lastQuarterEnd = today.minusDays(20);

        Map<String, Object> reported = new HashMap<>();
        reported.put("occurred-date", lastQuarterEnd.toString());
        reported.put("eps", 1.8);

        when(gateway.getEarningsReports(eq("AAPL"), anyString()))
                .thenReturn(List.of(reported));

        EarningsReport result = adapter.obtenerProximoEarnings("AAPL");

        // Next quarter = lastQuarterEnd + 3 months, announcement = + 35 days
        LocalDate nextQuarterEnd = lastQuarterEnd.plusMonths(3);
        long expectedDays = ChronoUnit.DAYS.between(today, nextQuarterEnd.plusDays(35));
        assertEquals(expectedDays, result.getDaysUntilEarnings());
        assertEquals(lastQuarterEnd, result.getOccurredDate());
    }

    @Test
    void obtenerProximoEarnings_parsesStringEps() {
        LocalDate today = LocalDate.now();

        Map<String, Object> reported = new HashMap<>();
        reported.put("occurred-date", today.minusDays(30).toString());
        reported.put("eps", "3.14");

        when(gateway.getEarningsReports(eq("AAPL"), anyString()))
                .thenReturn(List.of(reported));

        EarningsReport result = adapter.obtenerProximoEarnings("AAPL");

        assertEquals(3.14, result.getEps());
    }

    @Test
    void obtenerProximoEarnings_returnsMinusOneDays_whenNoReportedAndNoPending() {
        // Quarter with occurred-date but no eps and no reported quarters either
        // This case: all entries have null eps but we only have entries with null occurred-date
        // Actually let's test: reports where occurred-date is null
        Map<String, Object> badReport = new HashMap<>();
        badReport.put("occurred-date", null);
        badReport.put("eps", null);

        when(gateway.getEarningsReports(eq("AAPL"), anyString()))
                .thenReturn(List.of(badReport));

        EarningsReport result = adapter.obtenerProximoEarnings("AAPL");

        // No valid occurred-date, so both optionals are empty, daysUntil = -1
        assertEquals(-1L, result.getDaysUntilEarnings());
    }
}
