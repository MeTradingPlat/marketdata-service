package com.metradingplat.marketdata.domain.models;

public record VwapAccumulator(double cumulativePV, long lastDayVolume, double lastPrice, long updatedAtMillis) {

    public static VwapAccumulator seed(double price, long dayVolume, long nowMillis) {
        return new VwapAccumulator(price * dayVolume, dayVolume, price, nowMillis);
    }

    public VwapAccumulator withTrade(double price, long dayVolume, long nowMillis) {
        if (dayVolume < lastDayVolume) {
            return seed(price, dayVolume, nowMillis);
        }
        long delta = dayVolume - lastDayVolume;
        double newPV = delta > 0 ? cumulativePV + price * delta : cumulativePV;
        return new VwapAccumulator(newPV, dayVolume, price, nowMillis);
    }

    public double vwap() {
        return lastDayVolume > 0 ? cumulativePV / lastDayVolume : lastPrice;
    }
}
