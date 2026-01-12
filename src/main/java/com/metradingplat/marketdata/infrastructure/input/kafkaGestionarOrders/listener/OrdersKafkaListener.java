package com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.listener;

import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;
import com.metradingplat.marketdata.application.input.GestionarOrdersCUIntPort;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.DTOPetition.OrderRequestDTO;
import com.metradingplat.marketdata.infrastructure.input.kafkaGestionarOrders.mappers.OrdersKafkaMapper;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Component
@RequiredArgsConstructor
@Slf4j
public class OrdersKafkaListener {

    private final GestionarOrdersCUIntPort objGestionarOrdersCUInt;
    private final OrdersKafkaMapper objMapper;

    @KafkaListener(topics = "orders.commands", groupId = "marketdata-group")
    public void recibirComandoOrden(OrderRequestDTO command) {
        log.info("Recibido comando de orden para símbolo: {}", command.getSymbol());
        OrderRequest domainOrder = this.objMapper.deDTOADominio(command);
        this.objGestionarOrdersCUInt.placeBracketOrder(domainOrder);
    }
}
