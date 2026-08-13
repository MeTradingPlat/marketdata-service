package com.metradingplat.marketdata.infrastructure.input.health;

import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.CandleSubscriptionPool;
import lombok.RequiredArgsConstructor;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.util.List;

// Antes chequeaba un bean DxLinkClient que Spring crea por el @Component
// pero que nadie conecta -- ni el pool de fundamentals ni el de velas lo
// usan, cada uno crea sus propias conexiones con "new DxLinkClient()". Eso
// lo dejaba reportando DOWN siempre, sin importar si las conexiones reales
// estaban sanas (confirmado en vivo: velas respondiendo bien con esto en
// DOWN). Ahora chequea el pool de velas de verdad, igual que
// FundamentalsPoolHealthIndicator ya hace con el suyo.
@Component
@RequiredArgsConstructor
public class DxLinkHealthIndicator implements HealthIndicator {

    private final CandleSubscriptionPool candleSubscriptionPool;

    @Override
    public Health health() {
        List<Boolean> statuses = candleSubscriptionPool.connectionStatuses();
        long connected = statuses.stream().filter(Boolean::booleanValue).count();
        Health.Builder builder = (!statuses.isEmpty() && connected == 0) ? Health.down() : Health.up();
        return builder
                .withDetail("Candle Pool Connections", connected + "/" + statuses.size())
                .withDetail("Service", "MarketData")
                .build();
    }
}
