package handler

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/gorilla/websocket"
)

func TestIsPeerDisconnect(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"connection reset":  {&net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.ECONNRESET)}, true},
		"broken pipe":       {fmt.Errorf("write: %w", syscall.EPIPE), true},
		"closed connection": {net.ErrClosed, true},
		"close sent":        {websocket.ErrCloseSent, true},
		"going away":        {&websocket.CloseError{Code: websocket.CloseGoingAway}, true},
		"other failure":     {errors.New("json: unsupported type"), false},
	}
	for name, tc := range cases {
		if got := isPeerDisconnect(tc.err); got != tc.want {
			t.Errorf("%s: isPeerDisconnect = %v, want %v", name, got, tc.want)
		}
	}
}
