package com.metradingplat.marketdata.infrastructure.input.health;

import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.FundamentalsConnectionPool;
import lombok.RequiredArgsConstructor;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
@RequiredArgsConstructor
public class FundamentalsPoolHealthIndicator implements HealthIndicator {

    private final FundamentalsConnectionPool fundamentalsConnectionPool;

    @Override
    public Health health() {
        List<Boolean> statuses = fundamentalsConnectionPool.connectionStatuses();
        long connected = statuses.stream().filter(Boolean::booleanValue).count();
        // Conexiones individuales caidas se auto-recuperan via el backoff propio
        // de cada DxLinkClient -- solo se reporta DOWN si TODAS estan caidas.
        Health.Builder builder = (!statuses.isEmpty() && connected == 0) ? Health.down() : Health.up();
        return builder
                .withDetail("Fundamentals Pool Connections", connected + "/" + statuses.size())
                .withDetail("Service", "MarketData")
                .build();
    }
}
