package com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.mappers;

import org.mapstruct.Mapper;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.DTOPetition.OrderRequestDTO;

@Mapper(componentModel = "spring")
public interface OrdersKafkaMapper {
    OrderRequest deDTOADominio(OrderRequestDTO dto);
}
