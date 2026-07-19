package com.metradingplat.marketdata.adapter.external.dxlink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;

import java.io.IOException;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

/**
 * Unit tests for DxLinkWebSocketHandler.
 * Verifies message routing, protocol handshake, and error handling.
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("DxLinkWebSocketHandler Tests")
class DxLinkWebSocketHandlerTest {

    @Mock
    private DxLinkAdapter dxLinkAdapter;

    @Mock
    private WebSocketSession session;

    private DxLinkWebSocketHandler handler;
    private final ObjectMapper mapper = new ObjectMapper();

    @BeforeEach
    void setUp() {
        handler = new DxLinkWebSocketHandler(dxLinkAdapter);
        when(session.isOpen()).thenReturn(true);
    }

    // ─── Connection lifecycle ─────────────────────────────────────────────────

    @Test
    @DisplayName("shouldSendSetupMessageWhenConnectionIsEstablished")
    void shouldSendSetupMessageWhenConnectionIsEstablished() throws Exception {
        // Act
        handler.afterConnectionEstablished(session);

        // Assert — el primer mensaje enviado debe ser SETUP
        ArgumentCaptor<TextMessage> captor = ArgumentCaptor.forClass(TextMessage.class);
        verify(session).sendMessage(captor.capture());

        JsonNode setupMsg = mapper.readTree(captor.getValue().getPayload());
        assertThat(setupMsg.get("type").asText()).isEqualTo("SETUP");
        assertThat(setupMsg.get("keepaliveTimeout").asInt()).isEqualTo(60000);
        assertThat(setupMsg.get("version").get(0).asText()).isEqualTo("0.1");
    }

    @Test
    @DisplayName("shouldCloseSessionWithServerErrorWhenSetupMessageFails")
    void shouldCloseSessionWithServerErrorWhenSetupMessageFails() throws Exception {
        // Arrange — simular fallo al enviar
        when(session.isOpen()).thenReturn(true);
        org.mockito.Mockito.doThrow(new IOException("Network error"))
            .when(session).sendMessage(any());

        // Act
        handler.afterConnectionEstablished(session);

        // Assert — debe cerrar la sesión con SERVER_ERROR
        verify(session).close(CloseStatus.SERVER_ERROR);
    }

    // ─── Protocol message routing ─────────────────────────────────────────────

    @Test
    @DisplayName("shouldMarkHandshakeCompleteWhenChannelResponseIsReceived")
    void shouldMarkHandshakeCompleteWhenChannelResponseIsReceived() throws Exception {
        // Arrange
        assertThat(handler.isHandshakeComplete()).isFalse();
        TextMessage channelResponse = new TextMessage(
            "{\"type\":\"CHANNEL_RESPONSE\",\"channel\":0}");

        // Act
        handler.handleTextMessage(session, channelResponse);

        // Assert
        assertThat(handler.isHandshakeComplete()).isTrue();
    }

    @Test
    @DisplayName("shouldNotMarkHandshakeCompleteWhenFeedSetupIsReceived")
    void shouldNotMarkHandshakeCompleteWhenFeedSetupIsReceived() throws Exception {
        // Arrange
        TextMessage feedSetup = new TextMessage(
            "{\"type\":\"FEED_SETUP\",\"channel\":0}");

        // Act
        handler.handleTextMessage(session, feedSetup);

        // Assert — FEED_SETUP no completa el handshake
        assertThat(handler.isHandshakeComplete()).isFalse();
    }

    // ─── Data event routing ───────────────────────────────────────────────────

    @Test
    @DisplayName("shouldDelegateQuoteEventToAdapterWhenQuoteMessageIsReceived")
    void shouldDelegateQuoteEventToAdapterWhenQuoteMessageIsReceived() throws Exception {
        // Arrange
        TextMessage quoteMessage = new TextMessage(
            "{\"type\":\"Quote\",\"symbol\":\"SPY\","
            + "\"bid\":450.10,\"ask\":450.15,\"last\":450.12}");

        // Act
        handler.handleTextMessage(session, quoteMessage);

        // Assert
        verify(dxLinkAdapter).onDataEvent(eq("Quote"), any(JsonNode.class));
    }

    @Test
    @DisplayName("shouldDelegateGreeksEventToAdapterWhenGreeksMessageIsReceived")
    void shouldDelegateGreeksEventToAdapterWhenGreeksMessageIsReceived() throws Exception {
        // Arrange
        TextMessage greeksMessage = new TextMessage(
            "{\"type\":\"Greeks\",\"symbol\":\".SPY240119C450\","
            + "\"delta\":0.75,\"gamma\":0.02,\"theta\":-0.05,"
            + "\"vega\":0.10,\"rho\":0.15}");

        // Act
        handler.handleTextMessage(session, greeksMessage);

        // Assert
        verify(dxLinkAdapter).onDataEvent(eq("Greeks"), any(JsonNode.class));
    }

    @Test
    @DisplayName("shouldNotDelegateToAdapterWhenMessageTypeIsUnknown")
    void shouldNotDelegateToAdapterWhenMessageTypeIsUnknown() throws Exception {
        // Arrange — tipo desconocido que no es ni protocolo ni datos
        TextMessage unknownMessage = new TextMessage(
            "{\"type\":\"UNKNOWN_TYPE\",\"channel\":0}");

        // Act
        handler.handleTextMessage(session, unknownMessage);

        // Assert — no debe delegar al adapter
        verify(dxLinkAdapter, never()).onDataEvent(any(), any());
    }

    @Test
    @DisplayName("shouldNotThrowWhenMessagePayloadIsInvalidJson")
    void shouldNotThrowWhenMessagePayloadIsInvalidJson() throws Exception {
        // Arrange — JSON malformado
        TextMessage badMessage = new TextMessage("not-valid-json");

        // Act & Assert — no debe propagar excepción
        org.assertj.core.api.Assertions.assertThatCode(
            () -> handler.handleTextMessage(session, badMessage))
            .doesNotThrowAnyException();
    }

    // ─── Connection close & error ─────────────────────────────────────────────

    @Test
    @DisplayName("shouldNotifyAdapterAndResetHandshakeWhenConnectionIsClosed")
    void shouldNotifyAdapterAndResetHandshakeWhenConnectionIsClosed() throws Exception {
        // Arrange — simular handshake previo completado
        handler.setHandshakeComplete(true);

        // Act
        handler.afterConnectionClosed(session, CloseStatus.NORMAL);

        // Assert
        verify(dxLinkAdapter).onConnectionClosed();
        assertThat(handler.isHandshakeComplete()).isFalse();
    }

    @Test
    @DisplayName("shouldCloseSessionAndNotifyAdapterWhenTransportErrorOccurs")
    void shouldCloseSessionAndNotifyAdapterWhenTransportErrorOccurs() throws Exception {
        // Arrange
        RuntimeException transportError = new RuntimeException("Connection reset by peer");

        // Act
        handler.handleTransportError(session, transportError);

        // Assert
        verify(session).close(CloseStatus.SERVER_ERROR);
        verify(dxLinkAdapter).onConnectionError(transportError);
    }

    // ─── sendMessage ─────────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldSendMessageWhenSessionIsOpen")
    void shouldSendMessageWhenSessionIsOpen() throws Exception {
        // Arrange
        String payload = "{\"type\":\"KEEPALIVE\",\"channel\":0}";

        // Act
        handler.sendMessage(session, payload);

        // Assert
        ArgumentCaptor<TextMessage> captor = ArgumentCaptor.forClass(TextMessage.class);
        verify(session).sendMessage(captor.capture());
        assertThat(captor.getValue().getPayload()).isEqualTo(payload);
    }

    @Test
    @DisplayName("shouldNotSendMessageWhenSessionIsClosed")
    void shouldNotSendMessageWhenSessionIsClosed() throws Exception {
        // Arrange
        when(session.isOpen()).thenReturn(false);

        // Act
        handler.sendMessage(session, "{\"type\":\"KEEPALIVE\",\"channel\":0}");

        // Assert — no debe intentar enviar
        verify(session, never()).sendMessage(any());
    }
}
