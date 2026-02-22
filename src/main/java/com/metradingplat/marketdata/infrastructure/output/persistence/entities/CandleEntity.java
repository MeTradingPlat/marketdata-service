package com.metradingplat.marketdata.infrastructure.output.persistence.entities;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import jakarta.persistence.*;
import lombok.*;

import java.time.Instant;

@Entity
@Table(name = "candles_historicas", uniqueConstraints = {
        @UniqueConstraint(columnNames = { "symbol", "timeframe", "timestamp" })
}, indexes = {
        @Index(name = "idx_candle_symbol_tf_time", columnList = "symbol, timeframe, timestamp DESC")
})
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class CandleEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false, length = 20)
    private String symbol;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 10)
    private EnumTimeframe timeframe;

    @Column(nullable = false)
    private Instant timestamp;

    private Double open;
    private Double high;
    private Double low;
    private Double close;
    private Double volume;
}
