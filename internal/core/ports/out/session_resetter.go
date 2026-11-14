package out

import "context"

type SessionResetter interface {
	ResetSessions(ctx context.Context) error
}
