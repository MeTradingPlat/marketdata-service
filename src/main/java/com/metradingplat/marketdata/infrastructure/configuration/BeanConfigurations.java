package com.metradingplat.marketdata.infrastructure.configuration;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import com.metradingplat.marketdata.application.output.FundamentalsPersistenceGatewayIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.usecases.GestionarFundamentalsCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarHistoricalDataCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarMercadosCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarOrdersCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarQuoteCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarRealTimeCUAdapter;
import com.metradingplat.marketdata.domain.usecases.GestionarEarningsCUAdapter;
import com.metradingplat.marketdata.infrastructure.output.external.finviz.FinvizScraper;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;

@Configuration
public class BeanConfigurations {

    @Bean
    public GestionarHistoricalDataCUAdapter gestionarHistoricalDataCUIntPort(
            GestionarComunicacionExternalGatewayIntPort objExternalGateway) {
        return new GestionarHistoricalDataCUAdapter(objExternalGateway);
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

    @Bean
    public GestionarMercadosCUAdapter gestionarMercadosCUIntPort(
            GestionarComunicacionExternalGatewayIntPort objExternalGateway) {
        return new GestionarMercadosCUAdapter(objExternalGateway);
    }

    @Bean
    public GestionarQuoteCUAdapter gestionarQuoteCUIntPort(
            GestionarComunicacionExternalGatewayIntPort objExternalGateway) {
        return new GestionarQuoteCUAdapter(objExternalGateway);
    }

    @Bean
    public GestionarEarningsCUAdapter gestionarEarningsCUIntPort(
            GestionarComunicacionExternalGatewayIntPort objExternalGateway) {
        return new GestionarEarningsCUAdapter(objExternalGateway);
    }

    @Bean
    public GestionarFundamentalsCUAdapter gestionarFundamentalsCUIntPort(
            FundamentalsPersistenceGatewayIntPort persistenceGateway,
            FinvizScraper finvizScraper,
            TastyTradeService tastyTradeService) {
        return new GestionarFundamentalsCUAdapter(persistenceGateway, finvizScraper, tastyTradeService);
    }
}
