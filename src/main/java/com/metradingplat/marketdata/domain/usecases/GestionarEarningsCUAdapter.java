package com.metradingplat.marketdata.domain.usecases;

import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.List;
import java.util.Map;

import com.metradingplat.marketdata.application.input.GestionarEarningsCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.EarningsReport;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarEarningsCUAdapter implements GestionarEarningsCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    @Override
    public EarningsReport obtenerProximoEarnings(String symbol) {
        LocalDate today = LocalDate.now();
        String startDate = today.toString();

        List<Map<String, Object>> reports = this.objExternalGateway.getEarningsReports(symbol, startDate);

        if (reports.isEmpty()) {
            return EarningsReport.builder()
                    .symbol(symbol)
                    .daysUntilEarnings(-1L)
                    .build();
        }

        // Buscar el proximo earnings (fecha mas cercana >= hoy)
        LocalDate closestDate = null;
        Double eps = null;

        for (Map<String, Object> report : reports) {
            String dateStr = (String) report.get("occurred-date");
            if (dateStr == null) continue;

            LocalDate reportDate = LocalDate.parse(dateStr);
            if (!reportDate.isBefore(today)) {
                if (closestDate == null || reportDate.isBefore(closestDate)) {
                    closestDate = reportDate;
                    Object epsObj = report.get("eps");
                    if (epsObj instanceof Number) {
                        eps = ((Number) epsObj).doubleValue();
                    }
                }
            }
        }

        long daysUntil = closestDate != null ? ChronoUnit.DAYS.between(today, closestDate) : -1L;

        return EarningsReport.builder()
                .symbol(symbol)
                .occurredDate(closestDate)
                .eps(eps)
                .daysUntilEarnings(daysUntil)
                .build();
    }
}
