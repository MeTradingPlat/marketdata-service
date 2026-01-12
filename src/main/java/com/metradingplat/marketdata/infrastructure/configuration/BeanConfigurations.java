package com.metradingplat.marketdata.infrastructure.configuration;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.application.output.GestionarHistoricalDataGatewayIntPort;
import com.metradingplat.marketdata.domain.usecases.GestionarHistoricalDataCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarOrdersCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarRealTimeCUAdapter;

@Configuration
public class BeanConfigurations {

    @Bean
    public GestionarHistoricalDataCUAdapter gestionarHistoricalDataCUIntPort(
            GestionarHistoricalDataGatewayIntPort objObtenerHistoricalDataGateway,
            GestionarComunicacionExternalGatewayIntPort objExternalGateway) {
        return new GestionarHistoricalDataCUAdapter(objObtenerHistoricalDataGateway, objExternalGateway);
    }

    @Bean
    public GestionarOrdersCUAdapter gestionarOrdersCUIntPort(
            GestionarComunicacionExternalGatewayIntPort objGestionarComunicacionExterna) {
        return new GestionarOrdersCUAdapter(objGestionarComunicacionExterna);
    }

    @Bean
    public GestionarRealTimeCUAdapter gestionarRealTimeCUIntPort(
            GestionarComunicacionExternalGatewayIntPort objGestionarComunicacionExterna) {
        return new GestionarRealTimeCUAdapter(objGestionarComunicacionExterna);
    }
}
