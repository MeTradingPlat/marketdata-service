package com.metradingplat.marketdata.domain.models;

import java.math.BigDecimal;

import com.metradingplat.marketdata.domain.enums.EnumOrderAction;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * Orden bracket (OTOCO): orden principal + stop loss + take profit.
 * TastyTrade las maneja como One-Triggers-One-Cancels-Other.
 */
@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class BracketOrder {
    private String symbol;
    private EnumOrderAction action;
    private Integer quantity;
    private BigDecimal entryPrice;
    private BigDecimal stopLossPrice;
    private BigDecimal takeProfitPrice;
    private String timeInForce;
}
