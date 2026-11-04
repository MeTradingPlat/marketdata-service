package domain

import "errors"

var ErrPartialHistory = errors.New("history burst cut by timeout before settling")
