package com.metradingplat.marketdata.domain.enums;

import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

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

    EnumMercado(String code, String name, List<String> micCodes) {
        this.code = code;
        this.name = name;
        this.micCodes = micCodes;
    }

    /**
     * Acepta una mezcla de codigos "de usuario" (NYSE, NASDAQ, ETF...) y
     * codigos MIC reales (XNYS, XNAS, ARCX, BATS...) -- el frontend manda MIC
     * codes directamente (vienen de obtenerMercadosDisponibles(), que
     * descubre los MIC reales de TastyTrade), pero algun otro caller podria
     * seguir mandando los nombres cortos del enum. Antes se resolvia
     * "todo o nada": si NINGUN codigo coincidia con el enum, se asumian todos
     * MIC crudos; si UNO coincidia (ej. "OTC", que es igual en ambos
     * sistemas), el resto se descartaba en silencio -- con 6 MIC codes reales
     * donde solo "OTC" coincide por casualidad con el enum, el filtro
     * terminaba usando solo {"OTC"} y perdiendo los otros 5 mercados
     * completos.
     */
    public static Set<String> toMicCodes(Set<String> marketCodes) {
        Set<String> result = new HashSet<>();
        for (String code : marketCodes) {
            EnumMercado match = Arrays.stream(values()).filter(m -> m.code.equals(code)).findFirst().orElse(null);
            if (match != null) {
                result.addAll(match.micCodes);
            } else {
                result.add(code);
            }
        }
        return result;
    }
}
