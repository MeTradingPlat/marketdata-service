package httpretry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Options struct {
	Delays []time.Duration
	Header http.Header
	Wait   func(ctx context.Context) error
}

func Get(ctx context.Context, client *http.Client, url string, opts Options) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := getOnce(ctx, client, url, opts)
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil || !IsTransient(err) || attempt >= len(opts.Delays) {
			return nil, err
		}
		if sleepErr := Sleep(ctx, delayBefore(err, opts.Delays[attempt])); sleepErr != nil {
			return nil, err
		}
	}
}

func getOnce(ctx context.Context, client *http.Client, url string, opts Options) (*http.Response, error) {
	if opts.Wait != nil {
		if err := opts.Wait(ctx); err != nil {
			return nil, &permanentError{err: err}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &permanentError{err: fmt.Errorf("building request for %s: %w", url, err)}
	}
	for name, values := range opts.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		return resp, nil
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	return nil, &StatusError{URL: url, Code: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
}

func delayBefore(err error, base time.Duration) time.Duration {
	var status *StatusError
	if errors.As(err, &status) && status.RetryAfter > 0 {
		return status.RetryAfter
	}
	return base
}

func Sleep(ctx context.Context, wait time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}
