package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.concurrent.TimeUnit;

@Slf4j
@Component
public class CandleChannelOpener {

    // Abrir muchos canales nuevos de golpe no es confiable (ver
    // CandleWaveFetcher). Un stagger chico entre aperturas, mas un corte
    // temprano tras fallos seguidos, evita quedarse reintentando canales que
    // ya sabemos que no van a abrir (techo real de ~8 canales/conexion).
    private static final int MAX_CONSECUTIVE_FAILS = 2;
    private static final long OPEN_STAGGER_MS = 150;

    public List<DxLinkClient.DxLinkChannel> open(DxLinkClient client, int channelCount) throws InterruptedException {
        List<DxLinkClient.DxLinkChannel> opened = new ArrayList<>();
        int consecutiveFails = 0;
        for (int c = 0; c < channelCount; c++) {
            try {
                opened.add(client.openNewChannel(Set.of("Candle")).get(10, TimeUnit.SECONDS));
                consecutiveFails = 0;
            } catch (Exception e) {
                log.warn("Candle channel {}/{} failed to open: {}", c + 1, channelCount, e.getMessage());
                if (++consecutiveFails >= MAX_CONSECUTIVE_FAILS) {
                    log.warn("Stopping channel opening after {} consecutive failures, using {} opened channels",
                            consecutiveFails, opened.size());
                    break;
                }
            }
            if (c < channelCount - 1) Thread.sleep(OPEN_STAGGER_MS);
        }
        return opened;
    }
}
