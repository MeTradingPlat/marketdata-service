package com.metradingplat.marketdata.infrastructure.output.external.historicaldata;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestClient;

import lombok.Data;

@Configuration
@ConfigurationProperties(prefix = "historical-data-service")
@Data
public class HistoricalDataServiceConfig {

    private String url = "http://historical-data-service:8086";

    @Bean
    public RestClient historicalDataServiceRestClient() {
        return RestClient.builder()
                .baseUrl(url)
                .defaultHeader("Accept", "application/json")
                .build();
    }
}
