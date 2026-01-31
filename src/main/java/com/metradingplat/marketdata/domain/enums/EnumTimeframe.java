package com.metradingplat.marketdata.domain.enums;

import java.time.Duration;

import lombok.Getter;

@Getter
public enum EnumTimeframe {
    M1("1m", "{=1m}"),
    M5("5m", "{=5m}"),
    M15("15m", "{=15m}"),
    M30("30m", "{=30m}"),
    H1("1h", "{=1h}"),
    D1("1d", "{=1d}"),
    W1("1w", "{=1w}"),
    MO1("1mo", "{=1mo}");

    private final String label;
    private final String dxLinkFormat;

    EnumTimeframe(String label, String dxLinkFormat) {
        this.label = label;
        this.dxLinkFormat = dxLinkFormat;
    }

    public Duration getDuration() {
        return switch (this) {
            case M1  -> Duration.ofMinutes(1);
            case M5  -> Duration.ofMinutes(5);
            case M15 -> Duration.ofMinutes(15);
            case M30 -> Duration.ofMinutes(30);
            case H1  -> Duration.ofHours(1);
            case D1  -> Duration.ofDays(1);
            case W1  -> Duration.ofDays(7);
            case MO1 -> Duration.ofDays(30);
        };
    }
}