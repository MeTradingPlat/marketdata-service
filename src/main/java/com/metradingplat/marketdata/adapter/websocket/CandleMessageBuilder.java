package com.metradingplat.marketdata.adapter.websocket;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.metradingplat.marketdata.domain.models.Candle;

import java.util.List;

final class CandleMessageBuilder {

    private CandleMessageBuilder() {
    }

    static String history(ObjectMapper mapper, String symbol, String timeframe, List<Candle> bars) {
        ObjectNode root = mapper.createObjectNode();
        root.put("type", "history");
        root.put("symbol", symbol);
        root.put("timeframe", timeframe);
        ArrayNode arr = root.putArray("bars");
        for (Candle candle : bars) {
            arr.add(toBarNode(mapper, candle, true));
        }
        return root.toString();
    }

    static String bar(ObjectMapper mapper, String symbol, String timeframe, Candle candle, boolean closed) {
        ObjectNode root = mapper.createObjectNode();
        root.put("type", "bar");
        root.put("symbol", symbol);
        root.put("timeframe", timeframe);
        root.set("bar", toBarNode(mapper, candle, closed));
        return root.toString();
    }

    private static ObjectNode toBarNode(ObjectMapper mapper, Candle candle, boolean closed) {
        ObjectNode bar = mapper.createObjectNode();
        bar.put("time", candle.getTimestamp().getEpochSecond());
        bar.put("open", candle.getOpen());
        bar.put("high", candle.getHigh());
        bar.put("low", candle.getLow());
        bar.put("close", candle.getClose());
        bar.put("volume", candle.getVolume() != null ? candle.getVolume() : 0);
        bar.put("closed", closed);
        return bar;
    }
}
