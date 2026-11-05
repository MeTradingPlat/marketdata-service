package alerts

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func Send(ctx context.Context, level, message string, metadata map[string]any) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := os.Getenv("LOG_SERVICE_WS_URL")
	if url == "" {
		url = "ws://log-service:8084/ws/internal/logs"
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx2, url, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"servicioOrigen": "marketdata-service",
		"nivel":          level,
		"mensaje":        message,
		"categoria":      "MARKETDATA",
		"timestamp":      time.Now().UTC().Format("2006-01-02T15:04:05"),
		"metadatos":      string(metaBytes),
		"esSenalNueva":   true,
	})
	if err != nil {
		return err
	}

	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return err
	}
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	return conn.Close()
}

func Notify(level, message string, metadata map[string]any) {
	go func() {
		if err := Send(context.Background(), level, message, metadata); err != nil {
			log.Warn().Err(err).Str("alert", message).Msg("alert delivery to log-service failed")
		}
	}()
}
