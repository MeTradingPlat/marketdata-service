package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

@Component
public class CandleHistorySubscriber {

    public void subscribe(List<DxLinkClient.DxLinkChannel> channels, List<String> symbols, String period,
            String type, Instant fromTime) {
        int perChannel = (int) Math.ceil(symbols.size() / (double) channels.size());
        for (int c = 0; c < channels.size(); c++) {
            int groupStart = c * perChannel;
            if (groupStart >= symbols.size()) break;
            int groupEnd = Math.min(groupStart + perChannel, symbols.size());
            subscribeGroup(channels.get(c), symbols.subList(groupStart, groupEnd), period, type, fromTime);
        }
    }

    private void subscribeGroup(DxLinkClient.DxLinkChannel channel, List<String> group, String period, String type,
            Instant fromTime) {
        for (int i = 0; i < group.size(); i += 10) {
            int end = Math.min(i + 10, group.size());
            List<Map<String, Object>> items = new ArrayList<>();
            for (String symbol : group.subList(i, end)) {
                Map<String, Object> item = new HashMap<>();
                item.put("symbol", String.format("%s{=%s%s}", symbol, period, type));
                item.put("type", "Candle");
                items.add(item);
            }
            channel.subscribeCandlesHistory(items, fromTime.toEpochMilli());
        }
    }
}
