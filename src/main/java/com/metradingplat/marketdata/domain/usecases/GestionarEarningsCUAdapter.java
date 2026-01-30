package com.metradingplat.marketdata.domain.usecases;

import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Optional;

import com.metradingplat.marketdata.application.input.GestionarEarningsCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.EarningsReport;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@RequiredArgsConstructor
@Slf4j
public class GestionarEarningsCUAdapter implements GestionarEarningsCUIntPort {

    private static final int QUARTERLY_DAYS = 91;

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    @Override
    public EarningsReport obtenerProximoEarnings(String symbol) {
        LocalDate today = LocalDate.now();
        // TastyTrade solo tiene earnings historicos, buscar ultimo año
        String startDate = today.minusYears(1).toString();

        List<Map<String, Object>> reports = this.objExternalGateway.getEarningsReports(symbol, startDate);

        if (reports.isEmpty()) {
            log.warn("No earnings reports found for {}", symbol);
            return EarningsReport.builder()
                    .symbol(symbol)
                    .daysUntilEarnings(-1L)
                    .build();
        }

        // Encontrar el earnings mas reciente (fecha mas cercana a hoy)
        Optional<LocalDate> lastEarningsOpt = reports.stream()
                .map(r -> (String) r.get("occurred-date"))
                .filter(d -> d != null)
                .map(LocalDate::parse)
                .filter(d -> !d.isAfter(today))
                .max(Comparator.naturalOrder());

        if (lastEarningsOpt.isEmpty()) {
            return EarningsReport.builder()
                    .symbol(symbol)
                    .daysUntilEarnings(-1L)
                    .build();
        }

        LocalDate lastEarnings = lastEarningsOpt.get();

        // Obtener EPS del ultimo earnings
        Double eps = null;
        for (Map<String, Object> report : reports) {
            String dateStr = (String) report.get("occurred-date");
            if (dateStr != null && LocalDate.parse(dateStr).equals(lastEarnings)) {
                Object epsObj = report.get("eps");
                if (epsObj instanceof Number) {
                    eps = ((Number) epsObj).doubleValue();
                }
                break;
            }
        }

        // Estimar proximo earnings: ~91 dias despues del ultimo
        LocalDate estimatedNext = lastEarnings.plusDays(QUARTERLY_DAYS);
        long daysUntil = ChronoUnit.DAYS.between(today, estimatedNext);

        log.info("Earnings {}: ultimo={}, estimado proximo={}, dias={}", symbol, lastEarnings, estimatedNext, daysUntil);

        return EarningsReport.builder()
                .symbol(symbol)
                .occurredDate(lastEarnings)
                .eps(eps)
                .daysUntilEarnings(daysUntil)
                .build();
    }
}
