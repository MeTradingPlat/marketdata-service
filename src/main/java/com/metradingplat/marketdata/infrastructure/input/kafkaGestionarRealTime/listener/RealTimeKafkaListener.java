package com.metradingplat.marketdata.infrastructure.input.kafkaGestionarRealTime.listener;

import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;
import com.metradingplat.marketdata.application.input.GestionarRealTimeCUIntPort;
import com.metradingplat.marketdata.infrastructure.input.kafkaGestionarRealTime.DTOPetition.RealTimeRequestDTO;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Component
@RequiredArgsConstructor
@Slf4j
public class RealTimeKafkaListener {

    private final GestionarRealTimeCUIntPort objGestionarRealTimeCUInt;

    @KafkaListener(topics = "marketdata.commands", groupId = "marketdata-group")
    public void recibirComandoRealTime(RealTimeRequestDTO command) {
        log.info("Recibido comando RealTime: {} para símbolo: {}", command.getAction(), command.getSymbol());

        if ("SUBSCRIBE".equalsIgnoreCase(command.getAction())) {
            this.objGestionarRealTimeCUInt.subscribeToSymbol(command.getSymbol());
        } else if ("UNSUBSCRIBE".equalsIgnoreCase(command.getAction())) {
            this.objGestionarRealTimeCUInt.unsubscribeFromSymbol(command.getSymbol());
        } else {
            log.warn("Acción no reconocida: {}", command.getAction());
        }
    }
}
