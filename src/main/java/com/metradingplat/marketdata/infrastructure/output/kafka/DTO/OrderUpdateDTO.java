package com.metradingplat.marketdata.infrastructure.output.kafka.DTO;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class OrderUpdateDTO {
    private String orderId;
    private String symbol;
    private String status; // PLACED, FILLED, REJECTED, CANCELLED
    private Double price;
    private Double quantity;
    private String message;
}
