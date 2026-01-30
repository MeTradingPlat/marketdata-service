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

    private static final int EARNINGS_ANNOUNCEMENT_OFFSET = 35;

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    @Override
    public EarningsReport obtenerProximoEarnings(String symbol) {
        LocalDate today = LocalDate.now();
        String startDate = today.minusYears(2).toString();

        List<Map<String, Object>> reports = this.objExternalGateway.getEarningsReports(symbol, startDate);

        if (reports.isEmpty()) {
            log.warn("No earnings reports found for {}", symbol);
            return EarningsReport.builder()
                    .symbol(symbol)
                    .daysUntilEarnings(-1L)
                    .build();
        }

        // TastyTrade devuelve occurred-date = fin de trimestre fiscal, eps = null si aun no reportado
        // Buscar el ultimo trimestre CON eps (ya reportado)
        Optional<Map<String, Object>> lastReportedOpt = reports.stream()
                .filter(r -> r.get("occurred-date") != null && r.get("eps") != null)
                .max(Comparator.comparing(r -> LocalDate.parse((String) r.get("occurred-date"))));

        // Buscar el primer trimestre SIN eps (proximo earnings pendiente)
        Optional<Map<String, Object>> nextPendingOpt = reports.stream()
                .filter(r -> r.get("occurred-date") != null && r.get("eps") == null)
                .min(Comparator.comparing(r -> LocalDate.parse((String) r.get("occurred-date"))));

        Double eps = null;
        LocalDate lastReportedDate = null;

        if (lastReportedOpt.isPresent()) {
            Map<String, Object> lastReported = lastReportedOpt.get();
            lastReportedDate = LocalDate.parse((String) lastReported.get("occurred-date"));
            Object epsObj = lastReported.get("eps");
            if (epsObj instanceof Number) {
                eps = ((Number) epsObj).doubleValue();
            } else if (epsObj instanceof String) {
                try {
                    eps = Double.parseDouble((String) epsObj);
                } catch (NumberFormatException e) {
                    log.warn("Could not parse eps value: {}", epsObj);
                }
            }
        }

        // Calcular daysUntilEarnings
        long daysUntil;
        if (nextPendingOpt.isPresent()) {
            // Hay un trimestre pendiente: earnings se anuncian ~35 dias despues del fin del trimestre
            LocalDate pendingQuarterEnd = LocalDate.parse((String) nextPendingOpt.get().get("occurred-date"));
            LocalDate estimatedAnnouncement = pendingQuarterEnd.plusDays(EARNINGS_ANNOUNCEMENT_OFFSET);
            daysUntil = ChronoUnit.DAYS.between(today, estimatedAnnouncement);
            log.info("Earnings {}: ultimo reportado={} eps={}, pendiente trimestre={}, anuncio estimado={}, dias={}",
                    symbol, lastReportedDate, eps, pendingQuarterEnd, estimatedAnnouncement, daysUntil);
        } else if (lastReportedDate != null) {
            // Todos reportados, estimar siguiente trimestre
            LocalDate nextQuarterEnd = lastReportedDate.plusMonths(3);
            LocalDate estimatedAnnouncement = nextQuarterEnd.plusDays(EARNINGS_ANNOUNCEMENT_OFFSET);
            daysUntil = ChronoUnit.DAYS.between(today, estimatedAnnouncement);
            log.info("Earnings {}: ultimo reportado={} eps={}, estimado proximo anuncio={}, dias={}",
                    symbol, lastReportedDate, eps, estimatedAnnouncement, daysUntil);
        } else {
            daysUntil = -1L;
        }

        return EarningsReport.builder()
                .symbol(symbol)
                .occurredDate(lastReportedDate)
                .eps(eps)
                .daysUntilEarnings(daysUntil)
                .build();
    }
}
