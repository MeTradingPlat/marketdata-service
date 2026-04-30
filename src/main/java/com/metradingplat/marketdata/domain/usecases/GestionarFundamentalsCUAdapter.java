package com.metradingplat.marketdata.domain.usecases;

import com.metradingplat.marketdata.application.input.GestionarFundamentalsCUIntPort;
import com.metradingplat.marketdata.application.output.FundamentalsPersistenceGatewayIntPort;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.output.external.finviz.FinvizScraper;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

import java.time.Instant;
import java.time.LocalDate;
import java.time.ZoneOffset;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;

@Slf4j
@RequiredArgsConstructor
public class GestionarFundamentalsCUAdapter implements GestionarFundamentalsCUIntPort {

    private final FundamentalsPersistenceGatewayIntPort persistenceGateway;
    private final FinvizScraper finvizScraper;
    private final TastyTradeService tastyTradeService;

    // Control synchronization per symbol to avoid concurrent scrapes
    private final Map<String, Object> locks = new ConcurrentHashMap<>();

    @Override
    public FundamentalData obtenerFundamentals(String symbol) {
        Object lock = locks.computeIfAbsent(symbol, k -> new Object());

        synchronized (lock) {
            Optional<FundamentalData> cachedDataOpt = persistenceGateway.findBySymbol(symbol);
            FundamentalData data = cachedDataOpt.orElseGet(() -> FundamentalData.builder().symbol(symbol).build());

            boolean earningsStale = isEarningsStale(data);
            boolean shortInterestStale = isShortInterestStale(data);

            if (earningsStale || shortInterestStale) {
                log.info("Refreshing fundamentals for {}: earningsStale={}, shortInterestStale={}", 
                        symbol, earningsStale, shortInterestStale);
                
                Instant now = Instant.now();

                // 1. Refresh Earnings from TastyTrade if stale
                if (earningsStale) {
                    refreshEarningsFromTastyTrade(data);
                    data.setLastEarningsUpdated(now);
                }

                // 2. Refresh Short Interest from Finviz Scraper if stale
                if (shortInterestStale) {
                    FundamentalData scrapedData = finvizScraper.scrapeFundamentals(symbol);
                    if (scrapedData != null) {
                        data.setShortInterest(scrapedData.getShortInterest());
                        data.setShortRatio(scrapedData.getShortRatio());
                        // Fallback nextEarningsDate if TastyTrade failed
                        if (data.getNextEarningsDate() == null) {
                            data.setNextEarningsDate(scrapedData.getNextEarningsDate());
                        }
                    }
                    data.setLastShortInterestUpdated(now);
                }

                data.setLastUpdated(now);
                persistenceGateway.save(data);

                // Rate limiting delay for Finviz (only if we scraped)
                if (shortInterestStale) {
                    try {
                        Thread.sleep(1500);
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                    }
                }
            }

            // Merge with real-time data from Tastytrade/dxLink
            mergeRealTimeData(List.of(symbol), Map.of(symbol, data));

            return data;
        }
    }

    @Override
    public Map<String, FundamentalData> obtenerFundamentalsBatch(List<String> symbols) {
        Map<String, FundamentalData> resultMap = new ConcurrentHashMap<>();
        List<String> symbolsToScrape = new ArrayList<>();

        // 1. Initial check against DB
        for (String symbol : symbols) {
            Optional<FundamentalData> cached = persistenceGateway.findBySymbol(symbol);
            if (cached.isEmpty() || isEarningsStale(cached.get()) || isShortInterestStale(cached.get())) {
                symbolsToScrape.add(symbol);
            } else {
                resultMap.put(symbol, cached.get());
            }
        }

        // 2. Sequential/Batch refresh for missing/stale data
        for (String symbol : symbolsToScrape) {
            FundamentalData data = obtenerFundamentals(symbol);
            resultMap.put(symbol, data);
        }

        // 3. Final merge with real-time data for all symbols
        mergeRealTimeData(symbols, resultMap);

        return resultMap;
    }

    private boolean isEarningsStale(FundamentalData data) {
        if (data.getNextEarningsDate() == null || data.getLastEarningsUpdated() == null) {
            return true;
        }
        
        LocalDate todayUTC = LocalDate.now(ZoneOffset.UTC);
        if (data.getNextEarningsDate().isBefore(todayUTC) || data.getNextEarningsDate().isEqual(todayUTC)) {
            return true;
        }

        return data.getLastEarningsUpdated().isBefore(Instant.now().minus(24, ChronoUnit.HOURS));
    }

    private boolean isShortInterestStale(FundamentalData data) {
        if (data.getShortInterest() == null || data.getLastShortInterestUpdated() == null) {
            return true;
        }
        return data.getLastShortInterestUpdated().isBefore(Instant.now().minus(15, ChronoUnit.DAYS));
    }

    private void refreshEarningsFromTastyTrade(FundamentalData data) {
        try {
            String startDate = LocalDate.now().minusYears(1).toString();
            List<Map<String, Object>> reports = tastyTradeService.getEarningsReports(data.getSymbol(), startDate);
            
            if (reports != null && !reports.isEmpty()) {
                // Find latest reported quarter
                Optional<Map<String, Object>> lastReportedOpt = reports.stream()
                        .filter(r -> r.get("occurred-date") != null && r.get("eps") != null)
                        .max(Comparator.comparing(r -> LocalDate.parse((String) r.get("occurred-date"))));

                if (lastReportedOpt.isPresent()) {
                    Map<String, Object> lastReported = lastReportedOpt.get();
                    data.setOccurredDate(LocalDate.parse((String) lastReported.get("occurred-date")));
                    Object epsObj = lastReported.get("eps");
                    if (epsObj instanceof Number) {
                        data.setEps(((Number) epsObj).doubleValue());
                    }
                }

                // Find next pending quarter
                Optional<Map<String, Object>> nextPendingOpt = reports.stream()
                        .filter(r -> r.get("occurred-date") != null && r.get("eps") == null)
                        .min(Comparator.comparing(r -> LocalDate.parse((String) r.get("occurred-date"))));

                if (nextPendingOpt.isPresent()) {
                    LocalDate pendingQuarterEnd = LocalDate.parse((String) nextPendingOpt.get().get("occurred-date"));
                    // TastyTrade reports usually occur 30-40 days after quarter end if not yet scheduled
                    data.setNextEarningsDate(pendingQuarterEnd.plusDays(35)); 
                }
            }
        } catch (Exception e) {
            log.error("Error refreshing earnings from TastyTrade for {}: {}", data.getSymbol(), e.getMessage());
        }
    }

    private void mergeRealTimeData(List<String> symbols, Map<String, FundamentalData> mapToUpdate) {
        try {
            Map<String, FundamentalData> rtDataMap = tastyTradeService.getFundamentalsBatch(symbols);
            for (String sym : symbols) {
                FundamentalData existing = mapToUpdate.get(sym);
                FundamentalData rt = rtDataMap.get(sym);
                if (existing != null && rt != null) {
                    // Update real-time fields
                    existing.setMarketCap(rt.getMarketCap());
                    existing.setSharesOutstanding(rt.getSharesOutstanding());
                    existing.setFloatShares(rt.getFloatShares());
                    existing.setDayVolume(rt.getDayVolume());
                    existing.setPreMarketVolume(rt.getPreMarketVolume());
                    existing.setPostMarketVolume(rt.getPostMarketVolume());

                    // Prioritize nextEarningsDate from market-metrics if detailed fetch didn't happen or is old
                    if (rt.getNextEarningsDate() != null) {
                        existing.setNextEarningsDate(rt.getNextEarningsDate());
                    }

                    // Recalculate days until earnings
                    if (existing.getNextEarningsDate() != null) {
                        long days = ChronoUnit.DAYS.between(LocalDate.now(ZoneOffset.UTC), existing.getNextEarningsDate());
                        existing.setDaysUntilEarnings((int) Math.max(0, days));
                    }
                }
            }
        } catch (Exception e) {
            log.error("Error merging real-time fundamentals: {}", e.getMessage());
        }
    }
}
