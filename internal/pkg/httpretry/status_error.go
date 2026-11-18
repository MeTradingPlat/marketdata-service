package httpretry

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

type StatusError struct {
	URL        string
	Code       int
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s returned status %d", e.URL, e.Code)
}

func (e *StatusError) transient() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= http.StatusInternalServerError
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

func IsTransient(err error) bool {
	var permanent *permanentError
	if errors.As(err, &permanent) {
		return false
	}
	var status *StatusError
	if errors.As(err, &status) {
		return status.transient()
	}
	return true
}

func StatusCode(err error) int {
	var status *StatusError
	if errors.As(err, &status) {
		return status.Code
	}
	return 0
}
