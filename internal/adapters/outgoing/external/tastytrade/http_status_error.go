package tastytrade

import (
	"errors"
	"fmt"
	"net/http"
)

type httpStatusError struct {
	url    string
	status int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("%s returned status %d", e.url, e.status)
}

func isNotFoundError(err error) bool {
	var se *httpStatusError
	return errors.As(err, &se) && se.status == http.StatusNotFound
}
