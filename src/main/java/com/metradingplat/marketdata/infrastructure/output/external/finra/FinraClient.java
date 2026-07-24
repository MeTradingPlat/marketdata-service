package com.metradingplat.marketdata.infrastructure.output.external.finra;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URL;
import java.time.LocalDate;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import org.springframework.stereotype.Component;

import lombok.extern.slf4j.Slf4j;

@Slf4j
@Component
public class FinraClient {

    private static final String FINRA_URL = "https://cdn.finra.org/equity/otcmarket/biweekly/shrt%s.csv";
    private static final DateTimeFormatter DATE_FMT = DateTimeFormatter.ofPattern("yyyyMMdd");
    private static final int CANDIDATE_COUNT = 6;

    public static class ShortInterestRecord {
        public long sharesShorted;
        public long avgDailyVolume;
        public double daysToCover;
        public String settlementDate;
    }

    public Map<String, ShortInterestRecord> downloadLatest() {
        for (LocalDate candidate : recentSettlementDates(CANDIDATE_COUNT)) {
            Map<String, ShortInterestRecord> result = tryDownload(candidate);
            if (result != null) return result;
        }
        log.warn("No FINRA short interest data available after checking {} candidate settlement dates", CANDIDATE_COUNT);
        return Map.of();
    }

    private Map<String, ShortInterestRecord> tryDownload(LocalDate candidate) {
        String url = String.format(FINRA_URL, candidate.format(DATE_FMT));
        log.info("Trying FINRA short interest file: {}", url);
        try (BufferedReader reader = new BufferedReader(new InputStreamReader(new URL(url).openStream()))) {
            String header = reader.readLine();
            if (header == null || header.startsWith("<?xml")) return null;

            Map<String, ShortInterestRecord> result = new HashMap<>();
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
                result.put(symbol, rec);
            }
            log.info("Loaded {} short interest records from FINRA ({})", result.size(), candidate);
            return result;
        } catch (Exception e) {
            log.debug("FINRA file not available for {}: {}", candidate, e.getMessage());
            return null;
        }
    }

    private List<LocalDate> recentSettlementDates(int count) {
        List<LocalDate> dates = new ArrayList<>();
        LocalDate today = LocalDate.now();
        LocalDate cursor = today;
        for (int monthsBack = 0; monthsBack < count; monthsBack++) {
            LocalDate midMonth = nearestBusinessDay(
                    LocalDate.of(cursor.getYear(), cursor.getMonthValue(), Math.min(15, cursor.lengthOfMonth())), false);
            LocalDate endMonth = lastBusinessDayOfMonth(cursor);
            if (!midMonth.isAfter(today)) dates.add(midMonth);
            if (!endMonth.isAfter(today)) dates.add(endMonth);
            cursor = cursor.withDayOfMonth(1).minusDays(1);
        }
        return dates.stream()
                .distinct()
                .sorted(Comparator.reverseOrder())
                .limit(count)
                .toList();
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
