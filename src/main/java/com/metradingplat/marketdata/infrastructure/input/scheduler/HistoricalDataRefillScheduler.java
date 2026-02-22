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

    // Ejecutar cada sabado y domingo a las 02:00 AM
    @Scheduled(cron = "0 0 2 * * SAT,SUN")
    public void refillHistoricalData() {
        log.info("Starting weekend mass historical data refill process...");
        List<String> allSymbols = fetchAllActives();
        log.info("Discovered {} active symbols to refill. Starting phase.", allSymbols.size());

        // We segment into chunks of 50 to avoid overloading dxlink with a massive batch
        int batchSize = 50;
        for (EnumTimeframe tf : EnumTimeframe.values()) {
            log.info("Starting refill for timeframe: {}", tf);

            for (int i = 0; i < allSymbols.size(); i += batchSize) {
                int end = Math.min(i + batchSize, allSymbols.size());
                List<String> batch = allSymbols.subList(i, end);

                try {
                    log.info("Refilling batch {} to {} for TF {}", i, end, tf);
                    // This will hit the Cache-Aside DB Hybrid, which will find the gap,
                    // fetch from DxLink, save to PostgreSQL, and return.
                    // We request up to 1000 bars backward to ensure the DB gets fully populated
                    tastyTradeService.getCandlesBatch(batch, tf, 1000);

                    // Rate Limiting Protection (Anti-Ban)
                    // Sleep 5 seconds between batches to drain data softly over the weekend
                    Thread.sleep(5000);
                } catch (Exception e) {
                    log.error("Error fetching batch. Pausing briefly to avoid cascade failures.", e);
                    try {
                        Thread.sleep(10000);
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                    }
                }
            }
        }
        log.info("Weekend mass historical data refill completed successfully.");
    }

    private List<String> fetchAllActives() {
        List<String> symbols = new ArrayList<>();
        int offset = 0;
        int limit = 1000;

        while (true) {
            try {
                // Not all TT REST endpoints support pageOffset, but assuming generic pagination
                List<ActiveEquity> page = tastyTradeService.getActiveEquities(offset, limit);
                if (page == null || page.isEmpty()) {
                    break;
                }
                for (ActiveEquity equity : page) {
                    symbols.add(equity.getSymbol());
                }

                if (page.size() < limit)
                    break; // Finished

                offset += limit;

                // Be gentle with the TT REST API too
                Thread.sleep(500);
            } catch (Exception e) {
                log.error("Failed to fetch active equities chunk at offset {}", offset, e);
                break;
            }
        }
        return symbols;
    }
}
