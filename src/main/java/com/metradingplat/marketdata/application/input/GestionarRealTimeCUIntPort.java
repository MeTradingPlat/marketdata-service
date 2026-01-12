package com.metradingplat.marketdata.application.input;

public interface GestionarRealTimeCUIntPort {
    void subscribeToSymbol(String symbol);

    void unsubscribeFromSymbol(String symbol);
}
