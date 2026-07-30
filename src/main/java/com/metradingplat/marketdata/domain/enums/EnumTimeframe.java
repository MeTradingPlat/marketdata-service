package com.metradingplat.marketdata.domain.enums;

import java.time.Duration;

import lombok.Getter;

// Ampliado de 8 a 21 valores para igualar el set que ya usan
// scanner-management-service y signal-processing-service -- antes solo
// tenia M1/M5/M15/M30/H1/D1/W1/MO1, y configurar un filtro tecnico con
// cualquiera de los otros 13 (ej. 45M, 2H, 3D) fallaba en silencio en cada
// ciclo de escaneo (marketdata-service no reconocia ese timeframe). dxFeed
// soporta cualquier periodo numerico por protocolo (confirmado con su
// documentacion oficial de tipos de vela) -- format "{=<periodo><tipo>}"
// con tipo m/h/d/w/mo/y -- asi que ampliar aca es seguro, no un limite real
// de TastyTrade/dxFeed.
@Getter
public enum EnumTimeframe {
    M1("1m", "{=1m}"),
    M2("2m", "{=2m}"),
    M3("3m", "{=3m}"),
    M5("5m", "{=5m}"),
    M10("10m", "{=10m}"),
    M15("15m", "{=15m}"),
    M30("30m", "{=30m}"),
    M45("45m", "{=45m}"),
    H1("1h", "{=1h}"),
    H2("2h", "{=2h}"),
    H3("3h", "{=3h}"),
    H4("4h", "{=4h}"),
    H12("12h", "{=12h}"),
    D1("1d", "{=1d}"),
    D2("2d", "{=2d}"),
    D3("3d", "{=3d}"),
    W1("1w", "{=1w}"),
    MO1("1mo", "{=1mo}"),
    MO3("3mo", "{=3mo}"),
    MO6("6mo", "{=6mo}"),
    Y1("1y", "{=1y}");

    private final String label;
    private final String dxLinkFormat;

    EnumTimeframe(String label, String dxLinkFormat) {
        this.label = label;
        this.dxLinkFormat = dxLinkFormat;
    }

    public Duration getDuration() {
        return switch (this) {
            case M1  -> Duration.ofMinutes(1);
            case M2  -> Duration.ofMinutes(2);
            case M3  -> Duration.ofMinutes(3);
            case M5  -> Duration.ofMinutes(5);
            case M10 -> Duration.ofMinutes(10);
            case M15 -> Duration.ofMinutes(15);
            case M30 -> Duration.ofMinutes(30);
            case M45 -> Duration.ofMinutes(45);
            case H1  -> Duration.ofHours(1);
            case H2  -> Duration.ofHours(2);
            case H3  -> Duration.ofHours(3);
            case H4  -> Duration.ofHours(4);
            case H12 -> Duration.ofHours(12);
            case D1  -> Duration.ofDays(1);
            case D2  -> Duration.ofDays(2);
            case D3  -> Duration.ofDays(3);
            case W1  -> Duration.ofDays(7);
            case MO1 -> Duration.ofDays(30);
            case MO3 -> Duration.ofDays(90);
            case MO6 -> Duration.ofDays(180);
            case Y1  -> Duration.ofDays(365);
        };
    }
}
