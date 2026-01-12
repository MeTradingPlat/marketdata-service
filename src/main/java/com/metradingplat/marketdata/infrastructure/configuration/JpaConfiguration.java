package com.metradingplat.marketdata.infrastructure.configuration;

import java.util.Optional;

import org.springframework.boot.autoconfigure.domain.EntityScan;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.domain.AuditorAware;
import org.springframework.data.jpa.repository.config.EnableJpaAuditing;
import org.springframework.data.jpa.repository.config.EnableJpaRepositories;
import org.springframework.transaction.annotation.EnableTransactionManagement;

@Configuration
@EnableJpaAuditing
@EnableTransactionManagement
@EnableJpaRepositories(basePackages = "com.metradingplat.marketdata.infrastructure.output.persistence.repositories")
@EntityScan(basePackages = "com.metradingplat.marketdata.infrastructure.output.persistence.entitys")
public class JpaConfiguration {

    @Bean
    public AuditorAware<String> auditorProvider() {
        return () -> Optional.of("sistema");
    }
}
