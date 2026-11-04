package out

import "context"

type CompressionScheduler interface {
	SetCompressionScheduled(ctx context.Context, scheduled bool) error
}
