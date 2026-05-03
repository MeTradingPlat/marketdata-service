package com.metradingplat.marketdata.domain.models;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Builder;
import java.util.List;

/**
 * Modelos de dominio para órdenes complejas (OTOCO, OCO) según especificación de TastyTrade.
 */
public class ComplexOrderModels {

    @Builder
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record OrderLeg(
        @JsonProperty("instrument-type") String instrumentType,
        String symbol,
        int quantity,
        String action
    ) {}

    @Builder
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record SingleOrder(
        @JsonProperty("order-type") String orderType,
        @JsonProperty("time-in-force") String timeInForce,
        Double price,
        @JsonProperty("stop-trigger") Double stopTrigger,
        @JsonProperty("price-effect") String priceEffect,
        List<OrderLeg> legs
    ) {}

    @Builder
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record ComplexOrderRequest(
        String type, // OTOCO, OCO, etc.
        @JsonProperty("trigger-order") SingleOrder triggerOrder,
        List<SingleOrder> orders
    ) {}

    @Builder
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record DryRunResponse(
        @JsonProperty("buying-power-effect") BuyingPowerEffect buyingPowerEffect,
        String status,
        List<String> warnings
    ) {}

    @Builder
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record BuyingPowerEffect(
        @JsonProperty("change-in-buying-power") Double changeInBuyingPower,
        @JsonProperty("current-buying-power") Double currentBuyingPower,
        @JsonProperty("new-buying-power") Double newBuyingPower,
        @JsonProperty("buying-power-effect-sign") Integer buyingPowerEffectSign
    ) {}
}
