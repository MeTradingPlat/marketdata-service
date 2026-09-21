package tastytrade

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

var testPolicy = chunkPolicy{size: 2, attempts: 3}

func echoFetch(_ context.Context, symbols []string) ([]string, error) { return symbols, nil }

func TestFetchInChunks_SplitsBySizeAndKeepsOrder(t *testing.T) {
	var sizes []int
	fetch := func(ctx context.Context, symbols []string) ([]string, error) {
		sizes = append(sizes, len(symbols))
		return echoFetch(ctx, symbols)
	}

	got, err := fetchInChunks(context.Background(), []string{"A", "B", "C", "D", "E"}, testPolicy, fetch)

	if err != nil || !reflect.DeepEqual(got, []string{"A", "B", "C", "D", "E"}) || !reflect.DeepEqual(sizes, []int{2, 2, 1}) {
		t.Fatalf("got %v sizes %v err %v", got, sizes, err)
	}
}

func TestFetchInChunks_ATransientFailureIsRetriedAndSucceeds(t *testing.T) {
	calls := 0
	fetch := func(ctx context.Context, symbols []string) ([]string, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("timeout")
		}
		return echoFetch(ctx, symbols)
	}

	got, err := fetchInChunks(context.Background(), []string{"A", "B"}, testPolicy, fetch)

	if err != nil || len(got) != 2 || calls != 3 {
		t.Fatalf("got %v calls %d err %v", got, calls, err)
	}
}

func TestFetchInChunks_AChunkThatKeepsFailingDoesNotLoseTheOthers(t *testing.T) {
	fetch := func(ctx context.Context, symbols []string) ([]string, error) {
		if symbols[0] == "C" {
			return nil, errors.New("status 429")
		}
		return echoFetch(ctx, symbols)
	}

	got, err := fetchInChunks(context.Background(), []string{"A", "B", "C", "D", "E", "F"}, testPolicy, fetch)

	if err != nil || !reflect.DeepEqual(got, []string{"A", "B", "E", "F"}) {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestFetchInChunks_IfEveryChunkFailsItReturnsTheError(t *testing.T) {
	boom := errors.New("boom")
	fetch := func(context.Context, []string) ([]string, error) { return nil, boom }

	got, err := fetchInChunks(context.Background(), []string{"A", "B", "C"}, testPolicy, fetch)

	if got != nil || !errors.Is(err, boom) {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestFetchInChunks_ACanceledContextStopsTheLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	fetch := func(ctx context.Context, symbols []string) ([]string, error) {
		calls++
		cancel()
		return echoFetch(ctx, symbols)
	}

	_, err := fetchInChunks(ctx, []string{"A", "B", "C", "D"}, testPolicy, fetch)

	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err %v calls %d", err, calls)
	}
}

func TestFetchInChunks_EmptyInputReturnsNothing(t *testing.T) {
	got, err := fetchInChunks(context.Background(), nil, testPolicy, echoFetch)

	if err != nil || len(got) != 0 {
		t.Fatalf("got %v err %v", got, err)
	}
}
