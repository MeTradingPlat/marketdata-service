package com.metradingplat.marketdata.infrastructure.streaming;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.net.http.WebSocket;
import java.util.Map;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for EliteDxLinkStreamer
 * 
 * Tests FEED_SETUP and FEED_SUBSCRIPTION JSON payload validation
 * including Greeks and Underlying event support
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("EliteDxLinkStreamer - Greeks/Underlying Event Handling")
class EliteDxLinkStreamerTest {

    private EliteDxLinkStreamer streamer;
    private ObjectMapper objectMapper;
    
    @Mock
    private EliteDxLinkStreamer.MarketDataListener mockListener;
    
    @Mock
    private WebSocket mockWebSocket;

    @BeforeEach
    void setUp() {
        objectMapper = new ObjectMapper();
        streamer = new EliteDxLinkStreamer(
            "wss://stream.dxfeed.com/webapi",
            "test_quote_token",
            mockListener
        );
    }

    @Test
    @DisplayName("FEED_SETUP should include Greeks fields (volatility, delta, gamma, theta, vega)")
    void testFeedSetupIncludesGreeksFields() {
        // Arrange
        String expectedFeedSetup = "{\"type\":\"FEED_SETUP\",\"channel\":1," +
            "\"acceptDataFormat\":\"COMPACT\"," +
            "\"acceptEventFields\":{" +
            "\"Quote\":[\"eventSymbol\",\"bidPrice\",\"askPrice\",\"bidSize\",\"askSize\"]," +
            "\"Trade\":[\"eventSymbol\",\"price\",\"size\",\"time\"]," +
            "\"Greeks\":[\"eventSymbol\",\"volatility\",\"delta\",\"gamma\",\"theta\",\"vega\"]," +
            "\"Underlying\":[\"eventSymbol\",\"volatility\",\"putCallRatio\"]" +
            "}}";

        // Act
        streamer.subscribeSymbols(mockWebSocket, new String[]{"AAPL"});

        // Assert - Verify that FEED_SETUP contains Greeks fields
        verify(mockWebSocket, atLeastOnce()).sendText(argThat(msg -> 
            msg.contains("\"Greeks\"") && 
            msg.contains("\"volatility\"") &&
            msg.contains("\"delta\"") &&
            msg.contains("\"gamma\"") &&
            msg.contains("\"theta\"") &&
            msg.contains("\"vega\"")
        ), eq(true));
    }

    @Test
    @DisplayName("FEED_SETUP should include Underlying fields (volatility, putCallRatio)")
    void testFeedSetupIncludesUnderlyingFields() {
        // Act
        streamer.subscribeSymbols(mockWebSocket, new String[]{"AAPL"});

        // Assert - Verify that FEED_SETUP contains Underlying fields
        verify(mockWebSocket, atLeastOnce()).sendText(argThat(msg -> 
            msg.contains("\"Underlying\"") && 
            msg.contains("\"volatility\"") &&
            msg.contains("\"putCallRatio\"")
        ), eq(true));
    }

    @Test
    @DisplayName("FEED_SUBSCRIPTION should include Greeks symbol subscription")
    void testFeedSubscriptionIncludesGreeksSymbol() {
        // Act
        streamer.subscribeSymbols(mockWebSocket, new String[]{"AAPL"});

        // Assert - Verify that FEED_SUBSCRIPTION contains Greeks subscription
        verify(mockWebSocket, atLeastOnce()).sendText(argThat(msg -> 
            msg.contains("\"type\":\"Greeks\"") && 
            msg.contains(".AAPL261218C150")
        ), eq(true));
    }

    @Test
    @DisplayName("FEED_SUBSCRIPTION should include Underlying symbol subscription")
    void testFeedSubscriptionIncludesUnderlyingSymbol() {
        // Act
        streamer.subscribeSymbols(mockWebSocket, new String[]{"AAPL"});

        // Assert - Verify that FEED_SUBSCRIPTION contains Underlying subscription
        verify(mockWebSocket, atLeastOnce()).sendText(argThat(msg -> 
            msg.contains("\"type\":\"Underlying\"") && 
            msg.contains("\"symbol\":\"AAPL\"")
        ), eq(true));
    }

    @Test
    @DisplayName("MarketDataSnapshot should have Greeks fields")
    void testMarketDataSnapshotHasGreeksFields() {
        // Arrange
        EliteDxLinkStreamer.MarketDataSnapshot snapshot = new EliteDxLinkStreamer.MarketDataSnapshot();
        
        // Act
        snapshot.setSymbol("AAPL");
        snapshot.setType("GREEKS");
        snapshot.setVolatility(0.25);
        snapshot.setDelta(0.65);
        snapshot.setGamma(0.05);
        snapshot.setTheta(-0.02);
        snapshot.setVega(0.15);

        // Assert
        assertThat(snapshot.getSymbol()).isEqualTo("AAPL");
        assertThat(snapshot.getType()).isEqualTo("GREEKS");
        assertThat(snapshot.getVolatility()).isEqualTo(0.25);
        assertThat(snapshot.getDelta()).isEqualTo(0.65);
        assertThat(snapshot.getGamma()).isEqualTo(0.05);
        assertThat(snapshot.getTheta()).isEqualTo(-0.02);
        assertThat(snapshot.getVega()).isEqualTo(0.15);
    }

    @Test
    @DisplayName("MarketDataSnapshot should have Underlying fields")
    void testMarketDataSnapshotHasUnderlyingFields() {
        // Arrange
        EliteDxLinkStreamer.MarketDataSnapshot snapshot = new EliteDxLinkStreamer.MarketDataSnapshot();
        
        // Act
        snapshot.setSymbol("AAPL");
        snapshot.setType("UNDERLYING");
        snapshot.setVolatility(0.22);
        snapshot.setPutCallRatio(0.85);
        snapshot.setCallVolume(1500000L);
        snapshot.setPutVolume(1275000L);

        // Assert
        assertThat(snapshot.getSymbol()).isEqualTo("AAPL");
        assertThat(snapshot.getType()).isEqualTo("UNDERLYING");
        assertThat(snapshot.getVolatility()).isEqualTo(0.22);
        assertThat(snapshot.getPutCallRatio()).isEqualTo(0.85);
        assertThat(snapshot.getCallVolume()).isEqualTo(1500000L);
        assertThat(snapshot.getPutVolume()).isEqualTo(1275000L);
    }

    @Test
    @DisplayName("getAllSnapshots should return all cached snapshots")
    void testGetAllSnapshotsReturnsAllCachedSnapshots() {
        // Arrange
        EliteDxLinkStreamer.MarketDataSnapshot snapshot1 = new EliteDxLinkStreamer.MarketDataSnapshot();
        snapshot1.setSymbol("SPY");
        snapshot1.setType("QUOTE");
        snapshot1.setBidPrice(450.25);
        snapshot1.setAskPrice(450.26);

        EliteDxLinkStreamer.MarketDataSnapshot snapshot2 = new EliteDxLinkStreamer.MarketDataSnapshot();
        snapshot2.setSymbol("AAPL");
        snapshot2.setType("GREEKS");
        snapshot2.setDelta(0.65);

        // Act
        Map<String, EliteDxLinkStreamer.MarketDataSnapshot> allSnapshots = streamer.getAllSnapshots();

        // Assert
        assertThat(allSnapshots).isNotNull();
        assertThat(allSnapshots).isInstanceOf(Map.class);
    }

    @Test
    @DisplayName("JSON payloads should be valid and parseable")
    void testJsonPayloadsAreValid() throws Exception {
        // Arrange
        String feedSetup = "{\"type\":\"FEED_SETUP\",\"channel\":1," +
            "\"acceptDataFormat\":\"COMPACT\"," +
            "\"acceptEventFields\":{" +
            "\"Quote\":[\"eventSymbol\",\"bidPrice\",\"askPrice\",\"bidSize\",\"askSize\"]," +
            "\"Trade\":[\"eventSymbol\",\"price\",\"size\",\"time\"]," +
            "\"Greeks\":[\"eventSymbol\",\"volatility\",\"delta\",\"gamma\",\"theta\",\"vega\"]," +
            "\"Underlying\":[\"eventSymbol\",\"volatility\",\"putCallRatio\"]" +
            "}}";

        // Act & Assert - Should not throw exception
        assertThatNoException().isThrownBy(() -> {
            objectMapper.readTree(feedSetup);
        });
    }
}
