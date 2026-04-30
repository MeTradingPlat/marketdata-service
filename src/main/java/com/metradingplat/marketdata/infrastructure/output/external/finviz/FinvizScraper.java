package com.metradingplat.marketdata.infrastructure.output.external.finviz;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import lombok.extern.slf4j.Slf4j;
import org.jsoup.Jsoup;
import org.jsoup.nodes.Document;
import org.jsoup.select.Elements;
import org.springframework.stereotype.Component;

import java.time.LocalDate;
import java.time.format.DateTimeFormatter;
import java.time.format.DateTimeParseException;

@Slf4j
@Component
public class FinvizScraper {

    private static final String FINVIZ_URL = "https://finviz.com/quote.ashx?t=";

    public FundamentalData scrapeFundamentals(String symbol) {
        FundamentalData result = FundamentalData.builder().symbol(symbol).build();
        try {
            log.info("Scraping Finviz for symbol: {}", symbol);
            
            // Requerimiento de 1 segundo (puede ser manejado por quien llama, pero por si acaso)
            // Aquí lo dejamos libre de delay estricto, o podemos agregarlo. El usecase se encargará.

            Document doc = Jsoup.connect(FINVIZ_URL + symbol)
                    .userAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
                    .timeout(10000)
                    .get();

            Elements rows = doc.select("table.snapshot-table2 tr");
            rows.forEach(row -> {
                Elements cells = row.select("td");
                for (int i = 0; i < cells.size(); i += 2) {
                    if (i + 1 < cells.size()) {
                        String key = cells.get(i).text().trim();
                        String value = cells.get(i + 1).text().trim();
                        
                        switch (key) {
                            case "Short Float":
                                result.setShortInterest(parsePercentage(value));
                                break;
                            case "Short Ratio":
                                result.setShortRatio(parseNumber(value));
                                break;
                            case "Shs Outstand":
                                result.setSharesOutstanding(parseNumber(value) != null ? parseNumber(value).longValue() : null);
                                break;
                            case "Shs Float":
                                result.setFloatShares(parseNumber(value) != null ? parseNumber(value).longValue() : null);
                                break;
                            case "Earnings":
                                result.setNextEarningsDate(parseEarningsDate(value));
                                break;
                        }
                    }
                }
            });

        } catch (Exception e) {
            log.error("Finviz scraping failed for symbol {}: {}", symbol, e.getMessage());
        }
        return result;
    }

    private Double parseNumber(String value) {
        if (value == null || value.equals("-") || value.equals("N/A") || value.isEmpty()) {
            return null;
        }
        try {
            String clean = value.replace(",", "").toUpperCase();
            if (clean.endsWith("K")) return Double.parseDouble(clean.replace("K", "")) * 1_000;
            if (clean.endsWith("M")) return Double.parseDouble(clean.replace("M", "")) * 1_000_000;
            if (clean.endsWith("B")) return Double.parseDouble(clean.replace("B", "")) * 1_000_000_000;
            if (clean.endsWith("T")) return Double.parseDouble(clean.replace("T", "")) * 1_000_000_000_000L;
            return Double.parseDouble(clean);
        } catch (NumberFormatException e) {
            return null;
        }
    }

    private Double parsePercentage(String value) {
        if (value == null || value.equals("-") || value.equals("N/A") || value.isEmpty()) {
            return null;
        }
        try {
            return Double.parseDouble(value.replace("%", "").trim());
        } catch (NumberFormatException e) {
            return null;
        }
    }

    private LocalDate parseEarningsDate(String value) {
        if (value == null || value.equals("-") || value.isEmpty()) {
            return null;
        }
        try {
            // Finviz usually reports likes "Feb 28 AMC"
            String cleaned = value.replaceAll(" (AMC|BMO)", "").trim();
            // Assuming current year, handling crossover is tricky
            int currentYear = LocalDate.now().getYear();
            String dateStr = cleaned + " " + currentYear;
            DateTimeFormatter formatter = DateTimeFormatter.ofPattern("MMM dd yyyy", java.util.Locale.US);
            LocalDate date = LocalDate.parse(dateStr, formatter);
            
            // If date is too far in the past, it might be next year
            if (date.isBefore(LocalDate.now().minusMonths(6))) {
                date = date.plusYears(1);
            }
            return date;
        } catch (DateTimeParseException e) {
            return null;
        }
    }
}
