package com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.mappers;

import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.DTOPetition.OrderRequestDTO;
import javax.annotation.processing.Generated;
import org.springframework.stereotype.Component;

@Generated(
    value = "org.mapstruct.ap.MappingProcessor",
    date = "2026-01-15T21:04:22-0500",
    comments = "version: 1.5.5.Final, compiler: javac, environment: Java 21.0.9 (Eclipse Adoptium)"
)
@Component
public class OrdersKafkaMapperImpl implements OrdersKafkaMapper {

    @Override
    public OrderRequest deDTOADominio(OrderRequestDTO dto) {
        if ( dto == null ) {
            return null;
        }

        OrderRequest.OrderRequestBuilder orderRequest = OrderRequest.builder();

        orderRequest.symbol( dto.getSymbol() );
        orderRequest.action( dto.getAction() );
        orderRequest.type( dto.getType() );
        orderRequest.quantity( dto.getQuantity() );
        orderRequest.price( dto.getPrice() );
        orderRequest.stopLossPrice( dto.getStopLossPrice() );
        orderRequest.takeProfitPrice( dto.getTakeProfitPrice() );

        return orderRequest.build();
    }
}
