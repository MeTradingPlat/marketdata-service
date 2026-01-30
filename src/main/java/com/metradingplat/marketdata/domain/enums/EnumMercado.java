package com.metradingplat.marketdata.domain.enums;

import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

import lombok.Getter;

@Getter
public enum EnumMercado {
    NYSE("NYSE", "New York Stock Exchange", List.of("XNYS")),
    NASDAQ("NASDAQ", "NASDAQ", List.of("XNAS")),
    AMEX("AMEX", "NYSE American (AMEX)", List.of("XASE")),
    ETF("ETF", "Exchange Traded Funds", List.of("ARCX", "BATS")),
    OTC("OTC", "Over The Counter", List.of("OTC"));

    private final String code;
    private final String name;
    private final List<String> micCodes;

    private static final Map<String, EnumMercado> MIC_TO_MARKET;

    static {
        MIC_TO_MARKET = Arrays.stream(values())
                .flatMap(m -> m.micCodes.stream().map(mic -> Map.entry(mic, m)))
                .collect(Collectors.toMap(Map.Entry::getKey, Map.Entry::getValue));
    }

    EnumMercado(String code, String name, List<String> micCodes) {
        this.code = code;
        this.name = name;
        this.micCodes = micCodes;
    }

    /**
     * Given user-facing market codes (e.g. NYSE, NASDAQ), returns the set of
     * TastyTrade MIC codes (e.g. XNYS, XNAS) to filter by.
     */
    public static Set<String> toMicCodes(Set<String> marketCodes) {
        return Arrays.stream(values())
                .filter(m -> marketCodes.contains(m.code))
                .flatMap(m -> m.micCodes.stream())
                .collect(Collectors.toSet());
    }
}
