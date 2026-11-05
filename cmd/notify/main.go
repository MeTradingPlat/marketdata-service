package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/alerts"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "uso: notify <LEVEL> <message...>")
		os.Exit(2)
	}

	err := alerts.Send(context.Background(), os.Args[1], strings.Join(os.Args[2:], " "), map[string]any{"origen": "cd"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
