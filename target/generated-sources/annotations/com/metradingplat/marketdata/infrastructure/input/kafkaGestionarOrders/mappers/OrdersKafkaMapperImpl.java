package com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.mappers;

import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.DTOPetition.OrderRequestDTO;
import javax.annotation.processing.Generated;
import org.springframework.stereotype.Component;

@Generated(
    value = "org.mapstruct.ap.MappingProcessor",
    date = "2026-01-16T20:41:33-0500",
    comments = "version: 1.5.5.Final, compiler: Eclipse JDT (IDE) 3.45.0.v20260101-2150, environment: Java 21.0.9 (Eclipse Adoptium)"
)
@Component
public class OrdersKafkaMapperImpl implements OrdersKafkaMapper {

    @Override
    public OrderRequest deDTOADominio(OrderRequestDTO dto) {
        if ( dto == null ) {
            return null;
        }

        OrderRequest.OrderRequestBuilder orderRequest = OrderRequest.builder();

        orderRequest.action( dto.getAction() );
        orderRequest.price( dto.getPrice() );
        orderRequest.quantity( dto.getQuantity() );
        orderRequest.stopLossPrice( dto.getStopLossPrice() );
        orderRequest.symbol( dto.getSymbol() );
        orderRequest.takeProfitPrice( dto.getTakeProfitPrice() );
        orderRequest.type( dto.getType() );

        return orderRequest.build();
    }
}
