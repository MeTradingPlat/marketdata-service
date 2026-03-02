package com.metradingplat.marketdata.infrastructure.input.scheduler;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

@Component
@RequiredArgsConstructor
@Slf4j
public class HistoricalDataRefillScheduler {

    private final TastyTradeService tastyTradeService;

    private static final String SIMBOLO_PROBE = "SPY";
    private static final int BATCH_SIZE = 50;
    private static final int BARS_H1_INICIAL  = 1000; // ~41 días de H1
    private static final int BARS_W1_INICIAL  = 200;  // ~4 años de W1
    private static final int BARS_MO1_INICIAL = 120;  // ~10 años de MO1

    /**
     * Limpieza diaria a la 1:00 AM UTC.
     * Elimina de la DB los timeframes efímeros (M1, M5, M15, M30, D1).
     * Solo se conservan H1, W1 y MO1.
     */
    @Scheduled(cron = "0 0 1 * * *")
    public void limpiezaDiaria() {
        log.info("Limpieza diaria: eliminando candles de timeframes efímeros");
        try {
            tastyTradeService.limpiarTimeframesObsoletos();
        } catch (Exception e) {
            log.error("Error en limpieza diaria de candles", e);
        }
    }

    /**
     * Actualización semanal de H1 y W1: cada lunes a las 3:00 AM UTC.
     * Primero hace un probe rápido (1 símbolo, 1 barra) para confirmar que hay datos nuevos.
     * Si los hay, actualiza todos los símbolos activos.
     */
    @Scheduled(cron = "0 0 3 * * MON")
    public void actualizarH1yW1() {
        log.info("Actualizacion semanal H1/W1 iniciada");
        if (!tastyTradeService.hayNuevaBarraCompletada(SIMBOLO_PROBE, EnumTimeframe.H1)) {
            log.info("No hay nuevas barras H1 completadas. Saltando actualizacion semanal.");
            return;
        }
        List<String> symbols = fetchAllActives();
        log.info("Actualizando H1 para {} simbolos activos", symbols.size());
        actualizarEnLotes(symbols, EnumTimeframe.H1, BARS_H1_INICIAL);
        log.info("Actualizando W1 para {} simbolos activos", symbols.size());
        actualizarEnLotes(symbols, EnumTimeframe.W1, BARS_W1_INICIAL);
        log.info("Actualizacion semanal H1/W1 completada");
    }

    /**
     * Actualización mensual de MO1: el día 1 de cada mes a las 3:00 AM UTC.
     * Hace probe previo antes de actualizar todos los símbolos.
     */
    @Scheduled(cron = "0 0 3 1 * *")
    public void actualizarMO1() {
        log.info("Actualizacion mensual MO1 iniciada");
        if (!tastyTradeService.hayNuevaBarraCompletada(SIMBOLO_PROBE, EnumTimeframe.MO1)) {
            log.info("No hay nuevas barras MO1 completadas. Saltando actualizacion mensual.");
            return;
        }
        List<String> symbols = fetchAllActives();
        log.info("Actualizando MO1 para {} simbolos activos", symbols.size());
        actualizarEnLotes(symbols, EnumTimeframe.MO1, BARS_MO1_INICIAL);
        log.info("Actualizacion mensual MO1 completada");
    }

    private void actualizarEnLotes(List<String> symbols, EnumTimeframe timeframe, int barsHistorico) {
        for (int i = 0; i < symbols.size(); i += BATCH_SIZE) {
            List<String> batch = symbols.subList(i, Math.min(i + BATCH_SIZE, symbols.size()));
            try {
                log.info("Actualizando lote {}-{} para {}", i, i + batch.size(), timeframe);
                tastyTradeService.refreshCandles(batch, timeframe, barsHistorico);
                Thread.sleep(5000); // anti-ban
            } catch (InterruptedException ie) {
                Thread.currentThread().interrupt();
                break;
            } catch (Exception e) {
                log.error("Error en lote {}-{} para {}. Continuando...", i, i + batch.size(), timeframe, e);
                try {
                    Thread.sleep(10000);
                } catch (InterruptedException ie2) {
                    Thread.currentThread().interrupt();
                }
            }
        }
    }

    private List<String> fetchAllActives() {
        List<String> symbols = new ArrayList<>();
        int offset = 0;
        int limit = 1000;
        while (true) {
            try {
                List<ActiveEquity> page = tastyTradeService.getActiveEquities(offset, limit);
                if (page == null || page.isEmpty()) break;
                page.forEach(e -> symbols.add(e.getSymbol()));
                if (page.size() < limit) break;
                offset += limit;
                Thread.sleep(500);
            } catch (Exception e) {
                log.error("Error obteniendo activos en offset {}", offset, e);
                break;
            }
        }
        return symbols;
    }
}
