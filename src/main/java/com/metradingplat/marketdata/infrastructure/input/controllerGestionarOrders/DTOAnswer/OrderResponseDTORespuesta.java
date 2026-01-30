package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.DTOAnswer;

import java.time.OffsetDateTime;
import java.util.List;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class OrderResponseDTORespuesta {
    private String orderId;
    private String status;
    private OffsetDateTime receivedAt;
    private String complexOrderId;
    private String rejectReason;
    private List<String> warnings;
    private Double averageFillPrice;
}
