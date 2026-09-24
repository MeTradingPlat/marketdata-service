package handler

import (
	"errors"
	"net"
	"syscall"

	"github.com/gorilla/websocket"
)

func isPeerDisconnect(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, websocket.ErrCloseSent) ||
		websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseAbnormalClosure)
}
