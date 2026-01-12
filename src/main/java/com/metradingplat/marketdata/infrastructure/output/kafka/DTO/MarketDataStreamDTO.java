package com.metradingplat.marketdata.infrastructure.output.kafka.DTO;

import java.time.Instant;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class MarketDataStreamDTO {
    private String symbol;
    private Double lastPrice;
    private Double bid;
    private Double ask;
    private Long volume;
    private Instant timestamp;
}
