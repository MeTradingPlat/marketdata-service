package com.metradingplat.marketdata.domain.usecases;

import com.metradingplat.marketdata.application.input.GestionarFundamentalsCUIntPort;
import com.metradingplat.marketdata.application.output.FundamentalsPersistenceGatewayIntPort;
import com.metradingplat.marketdata.domain.models.FundamentalData;
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
    private final TastyTradeService tastyTradeService;

    private final Map<String, Object> locks = new ConcurrentHashMap<>();

    @Override
    public FundamentalData obtenerFundamentals(String symbol) {
        Object lock = locks.computeIfAbsent(symbol, k -> new Object());

        synchronized (lock) {
            Optional<FundamentalData> cachedDataOpt = persistenceGateway.findBySymbol(symbol);
            FundamentalData data = cachedDataOpt.orElseGet(() -> FundamentalData.builder().symbol(symbol).build());

            if (isEarningsStale(data)) {
                refreshEarningsFromTastyTrade(data);
                Instant now = Instant.now();
                data.setLastEarningsUpdated(now);
                data.setLastUpdated(now);
                persistenceGateway.save(data);
            }

            // Merge with real-time data from Tastytrade/dxLink (shortInterest/shortRatio
            // ya vienen de aca, poblados por FinraClient -- unico origen ahora)
            mergeRealTimeData(List.of(symbol), Map.of(symbol, data));

            return data;
        }
    }

    @Override
    public Map<String, FundamentalData> obtenerFundamentalsBatch(List<String> symbols) {
        Map<String, FundamentalData> resultMap = new ConcurrentHashMap<>();
        List<String> symbolsToRefresh = new ArrayList<>();

        // 1. Initial check against DB
        for (String symbol : symbols) {
            Optional<FundamentalData> cached = persistenceGateway.findBySymbol(symbol);
            if (cached.isEmpty() || isEarningsStale(cached.get())) {
                symbolsToRefresh.add(symbol);
            } else {
                resultMap.put(symbol, cached.get());
            }
        }

        // 2. Sequential/Batch refresh for missing/stale data
        for (String symbol : symbolsToRefresh) {
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
                    if (rt.getSharesOutstanding() != null && rt.getSharesOutstanding() > 0) existing.setSharesOutstanding(rt.getSharesOutstanding());
                    if (rt.getFloatShares() != null && rt.getFloatShares() > 0) existing.setFloatShares(rt.getFloatShares());
                    if (rt.getFloatSource() != null) existing.setFloatSource(rt.getFloatSource());
                    if (rt.getShortInterest() != null) existing.setShortInterest(rt.getShortInterest());
                    if (rt.getShortRatio() != null) existing.setShortRatio(rt.getShortRatio());
                    if (rt.getDayVolume() != null && rt.getDayVolume() > 0) existing.setDayVolume(rt.getDayVolume());
                    if (rt.getPreMarketVolume() != null && rt.getPreMarketVolume() > 0) existing.setPreMarketVolume(rt.getPreMarketVolume());
                    if (rt.getPostMarketVolume() != null && rt.getPostMarketVolume() > 0) existing.setPostMarketVolume(rt.getPostMarketVolume());
                    // Antes nunca se copiaban -- /marketdata/fundamentals/realtime (usado
                    // tanto por el popup de simbolo del frontend como por ChangeStrategy con
                    // PUNTO_REFERENCIA_CHANGE=CLOSE_PRE_MARKET/CLOSE_POST_MARKET en
                    // signal-processing-service) siempre devolvia null aca, aunque el dato
                    // ya estuviera bien poblado en TastyTradeService.fundamentalsCache.
                    if (rt.getPreMarketClose() != null) existing.setPreMarketClose(rt.getPreMarketClose());
                    if (rt.getPostMarketClose() != null) existing.setPostMarketClose(rt.getPostMarketClose());
                    
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
                    if (rt.getHigh52Week() != null) existing.setHigh52Week(rt.getHigh52Week());
                    if (rt.getLow52Week() != null) existing.setLow52Week(rt.getLow52Week());
                    if (rt.getNextExDividendDate() != null) existing.setNextExDividendDate(rt.getNextExDividendDate());
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
