package com.metradingplat.marketdata.adapter.external.dxlink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for DxLinkProtocol message builder.
 */
public class DxLinkProtocolTest {

    private ObjectMapper objectMapper;

    @BeforeEach
    public void setUp() {
        objectMapper = new ObjectMapper();
    }

    @Test
    public void shouldCreateSetupMessage() throws Exception {
        String message = DxLinkProtocol.createSetupMessage();
        
        assertNotNull(message);
        JsonNode node = objectMapper.readTree(message);
        
        assertEquals("SETUP", node.get("type").asText());
        assertEquals("0.1", node.get("version").get(0).asText());
        assertEquals(60000, node.get("keepaliveTimeout").asInt());
    }

    @Test
    public void shouldCreateAuthMessage() throws Exception {
        String token = "test-token-12345";
        String message = DxLinkProtocol.createAuthMessage(token);
        
        assertNotNull(message);
        JsonNode node = objectMapper.readTree(message);
        
        assertEquals("AUTH", node.get("type").asText());
        assertEquals(token, node.get("token").asText());
    }

    @Test
    public void shouldCreateChannelRequestMessage() throws Exception {
        String message = DxLinkProtocol.createChannelRequestMessage();
        
        assertNotNull(message);
        JsonNode node = objectMapper.readTree(message);
        
        assertEquals("CHANNEL_REQUEST", node.get("type").asText());
        assertEquals(0, node.get("channel").asInt());
        assertEquals("FEED", node.get("service").asText());
        assertEquals("AUTO", node.get("parameters").get("contract").asText());
    }

    @Test
    public void shouldCreateFeedSubscriptionMessage() throws Exception {
        List<String> symbols = Arrays.asList("AAPL", "MSFT", "GOOGL");
        String message = DxLinkProtocol.createFeedSubscriptionMessage(symbols);
        
        assertNotNull(message);
        JsonNode node = objectMapper.readTree(message);
        
        assertEquals("FEED_SUBSCRIPTION", node.get("type").asText());
        assertEquals(0, node.get("channel").asInt());
        assertEquals(3, node.get("add").size());
        assertEquals("AAPL", node.get("add").get(0).asText());
        assertEquals("MSFT", node.get("add").get(1).asText());
        assertEquals("GOOGL", node.get("add").get(2).asText());
    }

    @Test
    public void shouldCreateFeedUnsubscriptionMessage() throws Exception {
        List<String> symbols = Arrays.asList("AAPL");
        String message = DxLinkProtocol.createFeedUnsubscriptionMessage(symbols);
        
        assertNotNull(message);
        JsonNode node = objectMapper.readTree(message);
        
        assertEquals("FEED_SUBSCRIPTION", node.get("type").asText());
        assertEquals(0, node.get("channel").asInt());
        assertEquals(1, node.get("remove").size());
        assertEquals("AAPL", node.get("remove").get(0).asText());
    }

    @Test
    public void shouldCreateKeepaliveMessage() throws Exception {
        String message = DxLinkProtocol.createKeepaliveMessage();
        
        assertNotNull(message);
        JsonNode node = objectMapper.readTree(message);
        
        assertEquals("KEEPALIVE", node.get("type").asText());
        assertEquals(0, node.get("channel").asInt());
    }

    @Test
    public void shouldParseMessageType() throws Exception {
        String message = "{\"type\":\"Quote\",\"symbol\":\"AAPL\"}";
        String type = DxLinkProtocol.parseMessageType(message);
        
        assertEquals("Quote", type);
    }

    @Test
    public void shouldIdentifyResponseMessage() {
        assertTrue(DxLinkProtocol.isResponseMessage("CHANNEL_RESPONSE"));
        assertTrue(DxLinkProtocol.isResponseMessage("FEED_SETUP"));
        assertTrue(DxLinkProtocol.isResponseMessage("ERROR"));
        assertFalse(DxLinkProtocol.isResponseMessage("Quote"));
    }

    @Test
    public void shouldIdentifyDataEvent() {
        assertTrue(DxLinkProtocol.isDataEvent("Quote"));
        assertTrue(DxLinkProtocol.isDataEvent("Trade"));
        assertTrue(DxLinkProtocol.isDataEvent("Candle"));
        assertTrue(DxLinkProtocol.isDataEvent("Greeks"));
        assertTrue(DxLinkProtocol.isDataEvent("Underlying"));
        assertFalse(DxLinkProtocol.isDataEvent("SETUP"));
    }
}
