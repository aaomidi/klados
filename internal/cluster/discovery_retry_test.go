package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
)

func TestDiscoverLoop_EmitsPartialAndRetriesUntilFullSuccess(t *testing.T) {
	var emitted [][]APIResource
	calls := 0
	discover := func() ([]APIResource, error) {
		calls++
		if calls < 3 {
			return []APIResource{{GVR: "apps.v1.deployments"}}, fmt.Errorf("group xyz unavailable")
		}
		return []APIResource{{GVR: "apps.v1.deployments"}, {GVR: "xyz.v1.things"}}, nil
	}

	discoverLoop(context.Background(), time.Millisecond, 10*time.Millisecond, discover,
		func(r []APIResource) { emitted = append(emitted, r) })

	testza.AssertEqual(t, 3, calls)
	// partial results are emitted every round so the UI isn't empty during retry
	testza.AssertEqual(t, 3, len(emitted))
	testza.AssertEqual(t, 2, len(emitted[2]))
}

func TestDiscoverLoop_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	discover := func() ([]APIResource, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil, fmt.Errorf("still down")
	}
	discoverLoop(ctx, time.Millisecond, 10*time.Millisecond, discover, func([]APIResource) {})
	testza.AssertTrue(t, calls >= 2 && calls <= 3) // returns promptly after cancel
}

func TestDiscoverLoop_SuccessEmitsOnceEvenWhenEmpty(t *testing.T) {
	emits := 0
	discoverLoop(context.Background(), time.Millisecond, 10*time.Millisecond,
		func() ([]APIResource, error) { return nil, nil },
		func([]APIResource) { emits++ })
	testza.AssertEqual(t, 1, emits)
}
