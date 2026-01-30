package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.DTOPetition;

import java.math.BigDecimal;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Positive;
import lombok.AllArgsConstructor;
import lombok.Getter;
import lombok.NoArgsConstructor;
import lombok.Setter;

@Getter
@Setter
@NoArgsConstructor
@AllArgsConstructor
public class BracketOrderDTOPeticion {
    @NotBlank
    private String symbol;
    @NotBlank
    private String action;
    @NotNull
    @Positive
    private Integer quantity;
    @NotNull
    private BigDecimal entryPrice;
    @NotNull
    private BigDecimal stopLossPrice;
    @NotNull
    private BigDecimal takeProfitPrice;
    private String timeInForce;
}
