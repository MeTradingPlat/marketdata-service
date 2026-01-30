package com.metradingplat.marketdata.domain.enums;

import lombok.Getter;

@Getter
public enum EnumMercado {
    NYSE("NYSE", "New York Stock Exchange"),
    NASDAQ("NASDAQ", "NASDAQ"),
    AMEX("AMEX", "NYSE American (AMEX)"),
    ETF("ETF", "Exchange Traded Funds"),
    OTC("OTC", "Over The Counter");

    private final String code;
    private final String name;

    EnumMercado(String code, String name) {
        this.code = code;
        this.name = name;
    }
}
