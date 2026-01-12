package com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.DTOPetition;

import java.math.BigDecimal;
import com.metradingplat.marketdata.domain.enums.EnumOrderAction;
import com.metradingplat.marketdata.domain.enums.EnumOrderType;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class OrderRequestDTO {
    private String symbol;
    private EnumOrderAction action;
    private EnumOrderType type;
    private Integer quantity;
    private BigDecimal price;

    // Bracket (OTOCO) Fields
    private BigDecimal stopLossPrice;
    private BigDecimal takeProfitPrice;
}
