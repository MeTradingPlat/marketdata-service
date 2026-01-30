package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.mapper;

import org.mapstruct.Mapper;
import org.mapstruct.ReportingPolicy;

import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.OrderResponse;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.DTOAnswer.OrderResponseDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOrders.DTOPetition.BracketOrderDTOPeticion;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.IGNORE)
public interface OrdersRestMapper {

    BracketOrder dePeticionADominio(BracketOrderDTOPeticion peticion);

    OrderResponseDTORespuesta deDominioARespuesta(OrderResponse orderResponse);
}
