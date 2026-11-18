package tastytrade

import (
	"context"
	"errors"
	"testing"
)

type fakeAccessRefresher struct {
	failures int
	calls    int
}

func (f *fakeAccessRefresher) RefreshAccessToken(context.Context) (string, error) {
	f.calls++
	if f.calls <= f.failures {
		return "", errors.New("context deadline exceeded")
	}
	return "token", nil
}

type fakeQuoteRefresher struct{ calls int }

func (f *fakeQuoteRefresher) Refresh(context.Context) error {
	f.calls++
	return nil
}

func TestRefreshTokensRecoversFromTransientAccessFailure(t *testing.T) {
	access := &fakeAccessRefresher{failures: 2}
	quote := &fakeQuoteRefresher{}

	refreshTokens(context.Background(), access, quote, fastDelays)

	if access.calls != 3 {
		t.Fatalf("access calls = %d, want 3", access.calls)
	}
	if quote.calls != 1 {
		t.Fatalf("quote calls = %d, want 1", quote.calls)
	}
}

func TestRefreshTokensSkipsQuoteWhenAccessKeepsFailing(t *testing.T) {
	access := &fakeAccessRefresher{failures: 100}
	quote := &fakeQuoteRefresher{}

	refreshTokens(context.Background(), access, quote, fastDelays)

	if access.calls != len(fastDelays)+1 {
		t.Fatalf("access calls = %d, want %d", access.calls, len(fastDelays)+1)
	}
	if quote.calls != 0 {
		t.Fatalf("quote calls = %d, want 0", quote.calls)
	}
}
