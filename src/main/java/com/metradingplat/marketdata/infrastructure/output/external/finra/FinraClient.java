package com.metradingplat.marketdata.infrastructure.output.external.finra;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URL;
import java.time.LocalDate;
import java.time.format.DateTimeFormatter;
import java.util.HashMap;
import java.util.Map;
import java.util.stream.Collectors;

import org.springframework.stereotype.Component;

import lombok.extern.slf4j.Slf4j;

@Slf4j
@Component
public class FinraClient {

    private static final String FINRA_URL = "https://cdn.finra.org/equity/otcmarket/biweekly/shrt%s.csv";
    private static final DateTimeFormatter DATE_FMT = DateTimeFormatter.ofPattern("yyyyMMdd");
    private static final int MAX_RETRY_DAYS = 4;

    public static class ShortInterestRecord {
        public long sharesShorted;
        public long avgDailyVolume;
        public double daysToCover;
        public String settlementDate;
    }

    public Map<String, ShortInterestRecord> downloadLatest() {
        Map<String, ShortInterestRecord> result = new HashMap<>();
        String firstSettlementDate = null;

        for (int offset = 0; offset < MAX_RETRY_DAYS; offset++) {
            LocalDate candidate = findSettlementDate(offset);
            String url = String.format(FINRA_URL, candidate.format(DATE_FMT));
            log.info("Trying FINRA short interest file: {}", url);

            try (BufferedReader reader = new BufferedReader(
                    new InputStreamReader(new URL(url).openStream()))) {
                String header = reader.readLine();
                if (header == null) continue;

                String line;
                while ((line = reader.readLine()) != null) {
                    String[] cols = line.split("\\|", -1);
                    if (cols.length < 10) continue;

                    String symbol = cols[1].trim().toUpperCase();
                    if (symbol.isEmpty()) continue;

                    ShortInterestRecord rec = new ShortInterestRecord();
                    rec.sharesShorted = parseLong(cols[5]);
                    rec.avgDailyVolume = parseLong(cols[8]);
                    rec.daysToCover = parseDouble(cols[9]);
                    rec.settlementDate = cols.length > 13 ? cols[13].trim() : "";
                    if (firstSettlementDate == null) firstSettlementDate = rec.settlementDate;
                    result.put(symbol, rec);
                }

                log.info("Loaded {} short interest records from FINRA ({})", result.size(), candidate);
                return result;

            } catch (java.io.FileNotFoundException e) {
                log.debug("FINRA file not available for {} (offset {})", candidate, offset);
            } catch (Exception e) {
                log.warn("Failed to parse FINRA file for {}: {}", candidate, e.getMessage());
            }
        }

        log.warn("No FINRA short interest data available after {} attempts", MAX_RETRY_DAYS);
        return result;
    }

    private LocalDate findSettlementDate(int offsetDaysAgo) {
        LocalDate today = LocalDate.now().minusDays(offsetDaysAgo);
        LocalDate lastBizDay = lastBusinessDayOfMonth(today.getMonthValue() > 1
                ? today.withDayOfMonth(1).minusDays(1)
                : today);
        LocalDate midMonth = LocalDate.of(today.getYear(), today.getMonthValue(),
                Math.min(15, today.lengthOfMonth()));
        midMonth = nearestBusinessDay(midMonth, false);

        if (!today.isBefore(lastBizDay.plusDays(7))) return lastBizDay;
        if (!today.isBefore(midMonth.plusDays(7))) return midMonth;
        return lastBizDay;
    }

    private LocalDate lastBusinessDayOfMonth(LocalDate date) {
        LocalDate last = date.withDayOfMonth(date.lengthOfMonth());
        return nearestBusinessDay(last, true);
    }

    private LocalDate nearestBusinessDay(LocalDate date, boolean backwards) {
        while (date.getDayOfWeek().getValue() > 5) {
            date = backwards ? date.minusDays(1) : date.plusDays(1);
        }
        return date;
    }

    private long parseLong(String value) {
        if (value == null || value.isEmpty()) return 0L;
        try { return Long.parseLong(value); }
        catch (NumberFormatException e) { return 0L; }
    }

    private double parseDouble(String value) {
        if (value == null || value.isEmpty()) return 0.0;
        try { return Double.parseDouble(value); }
        catch (NumberFormatException e) { return 0.0; }
    }
}
