package com.metradingplat.marketdata.domain.models;

public record VwapQuote(double last, long volume, double vwap) {

    public static VwapQuote fromAccumulator(VwapAccumulator acc) {
        return new VwapQuote(acc.lastPrice(), acc.lastDayVolume(), acc.vwap());
    }
}
