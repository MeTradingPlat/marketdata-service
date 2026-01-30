package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.controller;

import org.springframework.http.ResponseEntity;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarOrdersCUIntPort;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.OrderResponse;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.DTOAnswer.OrderResponseDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.DTOPetition.BracketOrderDTOPeticion;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.mapper.OrdersRestMapper;

import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/marketdata/orders")
@RequiredArgsConstructor
@Validated
public class OrdersRestController {

    private final GestionarOrdersCUIntPort objGestionarOrdersCUInt;
    private final OrdersRestMapper objMapper;

    @PostMapping
    public ResponseEntity<OrderResponseDTORespuesta> placeBracketOrder(
            @RequestBody @Valid BracketOrderDTOPeticion peticion) {
        BracketOrder order = this.objMapper.dePeticionADominio(peticion);
        OrderResponse orderResponse = this.objGestionarOrdersCUInt.placeBracketOrderWithResponse(order);
        OrderResponseDTORespuesta respuesta = this.objMapper.deDominioARespuesta(orderResponse);
        return ResponseEntity.ok(respuesta);
    }

    @DeleteMapping("/{orderId}")
    public ResponseEntity<Void> cancelOrder(
            @PathVariable("orderId") @NotBlank String orderId) {
        this.objGestionarOrdersCUInt.cancelOrder(orderId);
        return ResponseEntity.noContent().build();
    }
}
