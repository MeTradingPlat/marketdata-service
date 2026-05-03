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
import java.util.Collections;
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
            String startDate = LocalDate.now().minusYears(2).toString(); // Pedimos 2 años para tener suficiente muestra
            List<Map<String, Object>> reports = tastyTradeService.getEarningsReports(data.getSymbol(), startDate);
            
            if (reports != null && reports.size() >= 2) {
                // 1. Extraer y ordenar todas las fechas de reportes pasados (los que tienen EPS)
                List<LocalDate> sortedDates = reports.stream()
                        .filter(r -> r.get("occurred-date") != null && r.get("eps") != null)
                        .map(r -> LocalDate.parse((String) r.get("occurred-date")))
                        .sorted()
                        .toList();

                if (sortedDates.isEmpty()) return;

                // 2. Guardar el último reporte conocido para el DTO
                LocalDate lastReportDate = sortedDates.get(sortedDates.size() - 1);
                data.setOccurredDate(lastReportDate);
                
                // Buscar el EPS del último reporte
                reports.stream()
                    .filter(r -> r.get("occurred-date") != null && LocalDate.parse((String) r.get("occurred-date")).equals(lastReportDate))
                    .findFirst()
                    .ifPresent(r -> {
                        Object epsObj = r.get("eps");
                        if (epsObj instanceof Number) data.setEps(((Number) epsObj).doubleValue());
                    });

                // 3. Algoritmo Predictivo: Calcular deltas entre reportes (mínimo 60 días para filtrar ruidos)
                List<Long> deltas = new ArrayList<>();
                for (int i = 1; i < sortedDates.size(); i++) {
                    long diff = ChronoUnit.DAYS.between(sortedDates.get(i - 1), sortedDates.get(i));
                    if (diff > 60) {
                        deltas.add(diff);
                    }
                }

                if (!deltas.isEmpty()) {
                    // Calculamos la MEDIANA para ser robustos a años bisiestos o retrasos puntuales
                    Collections.sort(deltas);
                    long medianDelta;
                    int size = deltas.size();
                    if (size % 2 == 0) {
                        medianDelta = (deltas.get(size / 2 - 1) + deltas.get(size / 2)) / 2;
                    } else {
                        medianDelta = deltas.get(size / 2);
                    }

                    // 4. Proyectar la próxima fecha: Último reporte + Mediana del Ciclo
                    LocalDate predictedDate = lastReportDate.plusDays(medianDelta);
                    
                    // Si la fecha predicha ya pasó, probablemente el reporte sea inminente (ventana de reporte)
                    // En ese caso, podríamos estimar el siguiente ciclo o marcarlo como "hoy"
                    data.setNextEarningsDate(predictedDate);
                    log.info("Predicted next earnings for {}: {} (based on {} day cycle)", 
                            data.getSymbol(), predictedDate, medianDelta);
                }
            }
        } catch (Exception e) {
            log.error("Error predicting earnings from TastyTrade for {}: {}", data.getSymbol(), e.getMessage());
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
                    
                    // New fields from dxLink
                    if (rt.getEps() != null) existing.setEps(rt.getEps());
                    if (rt.getDividendAmount() != null) existing.setDividendAmount(rt.getDividendAmount());
                    if (rt.getDividendFrequency() != null) existing.setDividendFrequency(rt.getDividendFrequency());
                    if (rt.getTradingStatus() != null) existing.setTradingStatus(rt.getTradingStatus());
                    if (rt.getStatusReason() != null) existing.setStatusReason(rt.getStatusReason());
                    if (rt.getHaltStartTime() != null) existing.setHaltStartTime(rt.getHaltStartTime());
                    if (rt.getHaltEndTime() != null) existing.setHaltEndTime(rt.getHaltEndTime());
                    if (rt.getBeta() != null) existing.setBeta(rt.getBeta());
                    if (rt.getOpen() != null) existing.setOpen(rt.getOpen());
                    if (rt.getHigh() != null) existing.setHigh(rt.getHigh());
                    if (rt.getLow() != null) existing.setLow(rt.getLow());
                    if (rt.getPrevClose() != null) existing.setPrevClose(rt.getPrevClose());
                    if (rt.getOpenInterest() != null) existing.setOpenInterest(rt.getOpenInterest());

                    // IV and Liquidity fields from Market Metrics
                    if (rt.getImpliedVolatilityIndex() != null) existing.setImpliedVolatilityIndex(rt.getImpliedVolatilityIndex());
                    if (rt.getImpliedVolatilityRank() != null) existing.setImpliedVolatilityRank(rt.getImpliedVolatilityRank());
                    if (rt.getImpliedVolatilityPercentile() != null) existing.setImpliedVolatilityPercentile(rt.getImpliedVolatilityPercentile());
                    if (rt.getLiquidity() != null) existing.setLiquidity(rt.getLiquidity());
                    if (rt.getLiquidityRating() != null) existing.setLiquidityRating(rt.getLiquidityRating());

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
