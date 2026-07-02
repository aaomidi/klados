package watcher_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/resource"
	"github.com/Vilsol/klados/internal/watcher"
)

type fakeProvider struct{}

func (f *fakeProvider) GetConnection(_ string) (*cluster.Connection, error) {
	return nil, fmt.Errorf("no connection")
}

func newTestManager(emit func(string, any)) *watcher.WatchManager {
	return watcher.NewWatchManager(
		&fakeProvider{},
		resource.NewEnricherRegistry(),
		emit,
		context.Background(),
	)
}

func TestWatchManager_StartStopLifecycle(t *testing.T) {
	mgr := newTestManager(func(string, any) {})

	err := mgr.StartWatch("ctx", "core.v1.pods", "default", "")
	testza.AssertNotNil(t, err) // expected: fakeProvider returns error

	mgr.StopAll()
}

func TestWatchManager_GraceTimerCancelled(t *testing.T) {
	var mu sync.Mutex
	emitted := 0
	mgr := newTestManager(func(_ string, _ any) {
		mu.Lock()
		emitted++
		mu.Unlock()
	})

	// StopWatch on a non-existent watch should be a no-op
	mgr.StopWatch("ctx", "core.v1.pods", "default")

	// Give grace timer time to not fire (it wasn't started)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	testza.AssertEqual(t, 0, emitted)
	mu.Unlock()
}

func TestWatchManager_StopAll(t *testing.T) {
	mgr := newTestManager(func(string, any) {})
	// StopAll on empty manager is safe
	mgr.StopAll()
}

func TestWatchManager_SyncEventConstants(t *testing.T) {
	testza.AssertEqual(t, "SYNC_START", watcher.EventSyncStart)
	testza.AssertEqual(t, "SYNC_END", watcher.EventSyncEnd)
}

type fakeVirtualSource struct {
	gotCtxName string
	gotNS      string
	gotRV      string
	emit       func(string, any)
	stopped    atomic.Bool
	wantErr    error
}

func (f *fakeVirtualSource) Watch(_ context.Context, contextName, namespace, rv string, emit func(string, any)) (func(), error) {
	if f.wantErr != nil {
		return nil, f.wantErr
	}
	f.gotCtxName = contextName
	f.gotNS = namespace
	f.gotRV = rv
	f.emit = emit
	return func() { f.stopped.Store(true) }, nil
}

func TestWatchManager_RegisterVirtual_Dispatch(t *testing.T) {
	var mu sync.Mutex
	emitted := map[string]any{}
	mgr := newTestManager(func(name string, payload any) {
		mu.Lock()
		emitted[name] = payload
		mu.Unlock()
	})

	src := &fakeVirtualSource{}
	mgr.RegisterVirtual("helm.v1.releases", src)

	err := mgr.StartWatch("my-ctx", "helm.v1.releases", "ns1", "rv-1")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "my-ctx", src.gotCtxName)
	testza.AssertEqual(t, "ns1", src.gotNS)
	testza.AssertEqual(t, "rv-1", src.gotRV)

	src.emit("watch:my-ctx:helm.v1.releases:ns1", map[string]string{"hello": "world"})
	mu.Lock()
	payload, ok := emitted["watch:my-ctx:helm.v1.releases:ns1"]
	mu.Unlock()
	testza.AssertTrue(t, ok)
	testza.AssertNotNil(t, payload)

	// StopAll must invoke the virtual source's stop callback.
	mgr.StopAll()
	testza.AssertTrue(t, src.stopped.Load())
}

func TestWatchManager_StopWatch_VirtualStopAfterGrace(t *testing.T) {
	// The grace-period callback path is the trickier teardown — verify it
	// invokes the virtual source's stop callback once the timer fires.
	orig := watcher.GracePeriodForTest()
	watcher.SetGracePeriodForTest(20 * time.Millisecond)
	defer watcher.SetGracePeriodForTest(orig)

	mgr := newTestManager(func(string, any) {})
	src := &fakeVirtualSource{}
	mgr.RegisterVirtual("helm.v1.releases", src)

	testza.AssertNoError(t, mgr.StartWatch("ctx", "helm.v1.releases", "ns", ""))
	mgr.StopWatch("ctx", "helm.v1.releases", "ns")

	// Stop callback not invoked yet — grace timer is still running.
	testza.AssertFalse(t, src.stopped.Load())

	// Wait for the grace period plus a small buffer.
	time.Sleep(80 * time.Millisecond)
	testza.AssertTrue(t, src.stopped.Load())
}

func TestWatchManager_MultipleVirtuals_NoConflict(t *testing.T) {
	mgr := newTestManager(func(string, any) {})
	a := &fakeVirtualSource{}
	b := &fakeVirtualSource{}
	mgr.RegisterVirtual("helm.v1.releases", a)
	mgr.RegisterVirtual("flux.v1.releases", b)

	testza.AssertNoError(t, mgr.StartWatch("ctx", "helm.v1.releases", "ns", ""))
	testza.AssertNoError(t, mgr.StartWatch("ctx", "flux.v1.releases", "ns", ""))

	testza.AssertEqual(t, "ns", a.gotNS)
	testza.AssertEqual(t, "ns", b.gotNS)

	mgr.StopAll()
	testza.AssertTrue(t, a.stopped.Load())
	testza.AssertTrue(t, b.stopped.Load())
}

func TestWatchManager_StartWatch_WithOptions_BackCompat(t *testing.T) {
	// Existing callers using empty-options form continue to compile and run.
	mgr := newTestManager(func(string, any) {})
	err := mgr.StartWatch("ctx", "core.v1.pods", "default", "", watcher.WatchOptions{FieldSelector: "metadata.name=foo"})
	// fakeProvider returns error — we're only verifying signature compat.
	testza.AssertNotNil(t, err)
}

type connProvider struct{ conn *cluster.Connection }

func (p *connProvider) GetConnection(string) (*cluster.Connection, error) { return p.conn, nil }

// A 410 Gone must tear down the watch map entry BEFORE emitting :resync, so the
// frontend's StartWatch-in-response actually starts a new watch goroutine.
func TestStartWatch_RestartsAfterGone(t *testing.T) {
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	dyn.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, nil, k8serrors.NewGone("gone")
	})
	conn := cluster.NewTestConnection(dyn, nil)
	defer conn.CloseForTest()

	resyncCh := make(chan struct{}, 8)
	mgr := watcher.NewWatchManager(&connProvider{conn}, resource.NewEnricherRegistry(), func(name string, _ any) {
		if strings.HasSuffix(name, ":resync") {
			resyncCh <- struct{}{}
		}
	}, context.Background())

	testza.AssertNoError(t, mgr.StartWatch("ctx", "core.v1.pods", "default", ""))
	select {
	case <-resyncCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first watch never hit Gone → resync")
	}

	// The frontend answers :resync with a fresh list + StartWatch. That second
	// StartWatch must spawn a NEW goroutine (which hits Gone again → resync).
	testza.AssertNoError(t, mgr.StartWatch("ctx", "core.v1.pods", "default", ""))
	select {
	case <-resyncCh:
	case <-time.After(2 * time.Second):
		t.Fatal("StartWatch after Gone was a no-op — stale map entry not cleaned up")
	}
}

// "Expired" (the reason real apiservers use for stale RVs) must be treated like
// Gone — resync, not an infinite same-RV retry loop.
func TestStartWatch_RestartsAfterExpired(t *testing.T) {
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	dyn.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, nil, k8serrors.NewResourceExpired("expired")
	})
	conn := cluster.NewTestConnection(dyn, nil)
	defer conn.CloseForTest()

	resyncCh := make(chan struct{}, 8)
	mgr := watcher.NewWatchManager(&connProvider{conn}, resource.NewEnricherRegistry(), func(name string, _ any) {
		if strings.HasSuffix(name, ":resync") {
			resyncCh <- struct{}{}
		}
	}, context.Background())

	testza.AssertNoError(t, mgr.StartWatch("ctx", "core.v1.pods", "default", ""))
	select {
	case <-resyncCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Expired error did not trigger resync")
	}
}

// Cancelling the connection context must terminate the watch goroutine and
// remove its map entry so a later StartWatch (post-reconnect) works.
func TestWatch_CleanedUpOnConnectionClose(t *testing.T) {
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	fw := watch.NewFake()
	defer fw.Stop()
	dyn.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, fw, nil
	})
	conn := cluster.NewTestConnection(dyn, nil)

	mgr := watcher.NewWatchManager(&connProvider{conn}, resource.NewEnricherRegistry(), func(string, any) {}, context.Background())
	testza.AssertNoError(t, mgr.StartWatch("ctx", "core.v1.pods", "default", ""))
	testza.AssertEqual(t, 1, mgr.WatchCountForTest())

	conn.CloseForTest()

	deadline := time.Now().Add(2 * time.Second)
	for mgr.WatchCountForTest() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	testza.AssertEqual(t, 0, mgr.WatchCountForTest())
}

func TestStopContext_StopsOnlyThatContext(t *testing.T) {
	mgr := newTestManager(func(string, any) {})
	a := &fakeVirtualSource{}
	mgr.RegisterVirtual("helm.v1.releases", a)

	testza.AssertNoError(t, mgr.StartWatch("ctx-a", "helm.v1.releases", "ns", ""))
	testza.AssertNoError(t, mgr.StartWatch("ctx-b", "helm.v1.releases", "ns", ""))
	testza.AssertEqual(t, 2, mgr.WatchCountForTest())

	mgr.StopContext("ctx-a")

	testza.AssertEqual(t, 1, mgr.WatchCountForTest())
	// ctx-b still runs: StartWatch for it is still a no-op
	testza.AssertNoError(t, mgr.StartWatch("ctx-b", "helm.v1.releases", "ns", ""))
	testza.AssertEqual(t, 1, mgr.WatchCountForTest())
}

func TestResyncContext_EmitsResyncPerWatch(t *testing.T) {
	var mu sync.Mutex
	var resyncs []string
	mgr := newTestManager(func(name string, _ any) {
		if strings.HasSuffix(name, ":resync") {
			mu.Lock()
			resyncs = append(resyncs, name)
			mu.Unlock()
		}
	})
	src := &fakeVirtualSource{}
	mgr.RegisterVirtual("helm.v1.releases", src)
	testza.AssertNoError(t, mgr.StartWatch("ctx", "helm.v1.releases", "ns1", ""))

	mgr.ResyncContext("ctx")

	mu.Lock()
	defer mu.Unlock()
	testza.AssertEqual(t, []string{"watch:ctx:helm.v1.releases:ns1:resync"}, resyncs)
	testza.AssertEqual(t, 0, mgr.WatchCountForTest())
}
