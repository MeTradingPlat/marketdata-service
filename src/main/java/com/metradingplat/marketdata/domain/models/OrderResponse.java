package com.metradingplat.marketdata.domain.models;

import lombok.Builder;
import lombok.Data;
import java.time.OffsetDateTime;
import java.util.List;

@Data
@Builder
public class OrderResponse {
    private String orderId; // ID principal de la orden (generado por Tastytrade)
    private String status; // RECEIVED, PLACED, FILLED, REJECTED
    private OffsetDateTime receivedAt; // Cuándo la recibió el broker

    // Para órdenes Bracket (OTOCO)
    private String complexOrderId; // ID que agrupa a la orden principal + SL + TP

    // Información de error (si status es REJECTED)
    private String rejectReason;
    private List<String> warnings; // Advertencias de Tastytrade (ej: "Falta poco para el cierre")

    // Detalle de precio final (si ya se ejecutó)
    private Double averageFillPrice;
}