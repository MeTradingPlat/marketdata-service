package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import jakarta.annotation.PostConstruct;

import java.time.LocalDate;
import java.time.format.DateTimeFormatter;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;

@Service
@RequiredArgsConstructor
@Slf4j
public class MarketTimeService {

    private final TastyTradeClient tastyTradeClient;

    // Cache local de las sesiones (se renueva mensualmente)
    private final ConcurrentHashMap<String, Map<String, Object>> sessionsCache = new ConcurrentHashMap<>();
    private List<Map<String, Object>> holidaysCache;

    @PostConstruct
    public void init() {
        log.info("Initializing MarketTimeService cache...");
        refreshCalendarCache();
    }

    /**
     * Tarea programada para actualizar el caché del calendario mensualmente.
     * Esto evita llamadas redundantes a la API de sesiones y asegura precisión de horarios.
     */
    @Scheduled(fixedRate = 30, timeUnit = TimeUnit.DAYS)
    public void refreshCalendarCache() {
        try {
            log.info("Refreshing market calendar cache (sessions and holidays)");
            
            // Traer 9 meses de historial (limite maximo del API)
            LocalDate now = LocalDate.now();
            String fromDate = now.minusMonths(1).format(DateTimeFormatter.ISO_LOCAL_DATE);
            String toDate = now.plusMonths(8).format(DateTimeFormatter.ISO_LOCAL_DATE);

            List<Map<String, Object>> sessions = tastyTradeClient.getMarketTimeSessions(fromDate, toDate);
            sessionsCache.clear();
            
            if (sessions != null) {
                for (Map<String, Object> session : sessions) {
                    if (session.containsKey("date")) {
                        sessionsCache.put((String) session.get("date"), session);
                    }
                }
            }
            
            holidaysCache = tastyTradeClient.getMarketTimeHolidays();
            
            log.info("Market calendar refreshed. Cached {} sessions and {} holidays.", sessionsCache.size(), holidaysCache != null ? holidaysCache.size() : 0);
        } catch (Exception e) {
            log.error("Failed to refresh market calendar cache", e);
        }
    }

    /**
     * Obtiene la sesión correspondiente a una fecha específica (formato YYYY-MM-DD).
     */
    public Map<String, Object> getSessionForDate(String dateStr) {
        return sessionsCache.get(dateStr);
    }
    
    /**
     * Valida si un rango de fechas de consulta excede el máximo permitido (9 meses).
     */
    public void validateHistoricalRange(LocalDate from, LocalDate to) {
        long monthsBetween = java.time.temporal.ChronoUnit.MONTHS.between(from, to);
        if (monthsBetween > 9) {
            throw new IllegalArgumentException("Intervalo histórico excede los 9 meses soportados por el API.");
        }
    }
}
