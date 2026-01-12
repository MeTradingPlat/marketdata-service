package com.metradingplat.marketdata.domain.models;

import java.math.BigDecimal;

import com.metradingplat.marketdata.domain.enums.EnumOrderAction;
import com.metradingplat.marketdata.domain.enums.EnumOrderType;

import lombok.Builder;
import lombok.Data;

@Data
@Builder
public class OrderRequest {
    private String symbol;
    private EnumOrderAction action;
    private EnumOrderType type;
    private Integer quantity;
    private BigDecimal price;

    // Bracket (OTOCO) Fields
    private BigDecimal stopLossPrice;
    private BigDecimal takeProfitPrice;
}