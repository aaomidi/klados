package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
)

func TestDiscoverWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	calls := 0
	res, err := discoverWithRetry(context.Background(), 5, time.Millisecond, func() ([]APIResource, error) {
		calls++
		if calls < 3 {
			return nil, fmt.Errorf("transient")
		}
		return []APIResource{{GVR: "core.v1.pods"}}, nil
	})

	testza.AssertNoError(t, err)
	testza.AssertEqual(t, 3, calls)
	testza.AssertEqual(t, 1, len(res))
}

func TestDiscoverWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	calls := 0
	_, err := discoverWithRetry(context.Background(), 3, time.Millisecond, func() ([]APIResource, error) {
		calls++
		return nil, fmt.Errorf("boom")
	})

	testza.AssertNotNil(t, err)
	testza.AssertEqual(t, 3, calls)
}

func TestDiscoverWithRetry_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	_, err := discoverWithRetry(ctx, 5, time.Second, func() ([]APIResource, error) {
		calls++
		return nil, fmt.Errorf("boom")
	})

	testza.AssertNotNil(t, err)
	testza.AssertEqual(t, 1, calls)
}
