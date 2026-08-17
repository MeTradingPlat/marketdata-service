package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	url := "ws://marketdata-service:8082/ws/candles"
	symbol := os.Getenv("SYMBOL")
	if symbol == "" {
		symbol = "MDXH"
	}
	tf := os.Getenv("TF")
	if tf == "" {
		tf = "M1"
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial failed:", err)
		os.Exit(1)
	}
	defer conn.Close()

	sub, _ := json.Marshal(map[string]string{"action": "subscribe", "symbol": symbol, "timeframe": tf})
	if err := conn.WriteMessage(websocket.TextMessage, sub); err != nil {
		fmt.Fprintln(os.Stderr, "subscribe failed:", err)
		os.Exit(1)
	}

	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var historyBars int
	var firstBar, lastBar int64
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			fmt.Fprintln(os.Stderr, "read failed:", err)
			break
		}
		var msg struct {
			Type string `json:"type"`
			Bars []struct {
				Time int64 `json:"time"`
			} `json:"bars"`
			Bar *struct {
				Time int64 `json:"time"`
			} `json:"bar"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			fmt.Println("raw:", string(raw))
			continue
		}
		switch msg.Type {
		case "history":
			historyBars = len(msg.Bars)
			if len(msg.Bars) > 0 {
				firstBar = msg.Bars[0].Time
				lastBar = msg.Bars[len(msg.Bars)-1].Time
			}
			fmt.Printf("history: %d bars, first=%s last=%s\n", historyBars,
				time.Unix(firstBar, 0).UTC().Format(time.RFC3339),
				time.Unix(lastBar, 0).UTC().Format(time.RFC3339))
			return
		case "bar":
			fmt.Println("live bar at", time.Unix(msg.Bar.Time, 0).UTC().Format(time.RFC3339))
			return
		case "error":
			fmt.Println("error message:", string(raw))
			return
		}
	}
}
