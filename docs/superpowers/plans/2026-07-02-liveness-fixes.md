# Liveness Fixes (Critical + High) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix findings #1–#9 from `docs/2026-07-02-liveness-audit.md` so the app recovers from network outages, laptop sleep, cert rotation, and kubeconfig changes without a restart.

**Architecture:** Watches become self-cleaning (map entry removed when the goroutine exits) and connection-scoped (derived from the connection's context). The cluster Manager gains disconnect/recovery hooks so the WatchManager tears down or resyncs watches on connection transitions. Health probes get per-call timeouts. Discovery retries until success and a CRD watch triggers re-discovery. Kubeconfig files are fsnotify-watched with credential-hash diffing → reconnect. The frontend re-lists on `error/disconnected → connected` transitions and handles the namespace-watch `:resync` protocol.

**Tech Stack:** Go (client-go, dynamic/fake for tests, testza, fsnotify), Svelte 5 runes stores, vitest.

**Conventions that apply to every task:**
- Go tests live in the same style as existing ones. `internal/watcher` tests are external (`package watcher_test`); `internal/cluster` tests are internal (`package cluster`) — check the specific file before adding.
- Logging: `slox.Info/Warn/Debug(ctx, msg, k, v...)` — never `*slog.Logger`.
- Commit after each task with the jj-vcs skill (`jj st` → `jj new` if @ has other changes → `jj desc -m "..."`). Never leave a task uncommitted.
- Run gate per task: `go test ./internal/cluster/ ./internal/watcher/ ./internal/resource/` (fast, no CGO). Frontend tasks: `cd frontend && pnpm check && pnpm test`.
- No Wails binding regen is needed — no bound service method signatures change in this plan.

---

### Task 1: Watch self-cleanup + connection-scoped contexts (findings #1, #2 core)

The bug: `runWatch` exits on 410 Gone (or connection death) but leaves `m.watches[key]` behind, so `StartWatch` no-ops forever (`internal/watcher/manager.go:118-124`). Watches also run on `context.Background()` and outlive their connection.

**Files:**
- Modify: `internal/cluster/manager.go` (add `Connection.Context()` accessor)
- Modify: `internal/cluster/testing.go` (test connection constructor)
- Modify: `internal/watcher/manager.go`
- Test: `internal/watcher/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/watcher/manager_test.go`:

```go
import (
	// add to existing imports:
	"strings"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

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
```

Add to `internal/cluster/testing.go`:

```go
import (
	"context"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

// NewTestConnection builds a Connection with a live connCtx for unit tests.
// disc may be nil when the test doesn't exercise discovery.
func NewTestConnection(dyn dynamic.Interface, disc discovery.DiscoveryInterface) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connection{Dynamic: dyn, Discovery: disc, connCtx: ctx, cancel: cancel}
}

// CloseForTest cancels the test connection's context (simulates Disconnect).
func (c *Connection) CloseForTest() {
	if c.cancel != nil {
		c.cancel()
	}
}
```

Add to `internal/watcher/manager.go` (or a new `internal/watcher/testing.go`):

```go
// WatchCountForTest returns the number of tracked watches. Test-only helper.
func (m *WatchManager) WatchCountForTest() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.watches)
}
```

- [ ] **Step 2: Run to verify red**

Run: `go test ./internal/watcher/ -run 'TestStartWatch_RestartsAfterGone|TestStartWatch_RestartsAfterExpired|TestWatch_CleanedUpOnConnectionClose' -v -timeout 60s`
Expected: all three FAIL (timeouts waiting for second resync / count never reaches 0). `TestStartWatch_RestartsAfterExpired` fails differently — it loops retrying with a 5s backoff and never resyncs.

- [ ] **Step 3: Implement**

In `internal/cluster/manager.go`, next to the `Connection` struct:

```go
// Context returns the connection-scoped context, cancelled on Disconnect.
// Anything whose lifetime should not outlive the connection (watches, streams)
// must derive from it.
func (c *Connection) Context() context.Context {
	return c.connCtx
}
```

In `internal/watcher/manager.go`:

1. `StartWatch` — derive from the connection context and capture the state pointer (replace lines 168-172):

```go
	ctx, cancel := context.WithCancel(conn.Context())
	state := &watchState{cancel: cancel}
	m.watches[key] = state

	go m.runWatch(ctx, state, ri, enrichers, eventName, resyncName, key, contextName, resourceVersion, opt.FieldSelector)
	return nil
```

2. Add ownership-checked cleanup (pointer identity prevents deleting a successor watch that reused the key):

```go
// removeSelf deletes the map entry for key iff it still belongs to state.
// Idempotent; safe to call from both the Gone path and the deferred exit path.
func (m *WatchManager) removeSelf(key watchKey, state *watchState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.watches[key]; ok && s == state {
		if s.graceTimer != nil {
			s.graceTimer.Stop()
		}
		delete(m.watches, key)
	}
}
```

3. `runWatch` — take the state, clean up on every exit, clean up *before* emitting resync (so the frontend's immediate StartWatch finds the key free), and treat Expired like Gone:

```go
func (m *WatchManager) runWatch(
	ctx context.Context,
	state *watchState,
	ri dynamic.ResourceInterface,
	enrichers []resource.Enricher,
	eventName string,
	resyncName string,
	key watchKey,
	contextName string,
	initialRV string,
	fieldSelector string,
) {
	defer m.removeSelf(key, state)

	currentRV := initialRV
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wi, err := ri.Watch(ctx, metav1.ListOptions{
			ResourceVersion:     currentRV,
			AllowWatchBookmarks: true,
			FieldSelector:       fieldSelector,
		})
		if err != nil {
			if k8serrors.IsGone(err) || k8serrors.IsResourceExpired(err) {
				slox.Warn(m.ctx, "watch RV too old, requesting resync", "event", eventName, "rv", currentRV)
				m.removeSelf(key, state)
				m.emitEvent(resyncName, nil)
				return
			}
			slox.Warn(m.ctx, "watch failed, retrying", "event", eventName, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		nextRV, gone := m.processEvents(ctx, wi, enrichers, eventName, contextName, currentRV)
		if gone {
			slox.Warn(m.ctx, "watch stream returned Gone, requesting resync", "event", eventName, "rv", currentRV)
			m.removeSelf(key, state)
			m.emitEvent(resyncName, nil)
			return
		}
		currentRV = nextRV
	}
}
```

- [ ] **Step 4: Run to verify green**

Run: `go test ./internal/watcher/ -v -timeout 120s` — all tests pass, including the pre-existing ones (grace timer, virtuals).
Also run: `go test ./internal/cluster/ ./internal/resource/ -timeout 120s`

- [ ] **Step 5: Commit** — `jj desc -m "fix(watcher): clean up watch map entries on exit and scope watches to the connection context"` (jj-vcs skill flow).

---

### Task 2: StopContext/ResyncContext + cluster disconnect/recovery hooks (finding #2 completion, backend half of #7)

Disconnect must synchronously tear down that context's watches (no reliance on async ctx propagation), and health-monitor recovery must force a resync of every watch (a wedged watch on a half-open socket never notices recovery on its own).

**Files:**
- Modify: `internal/watcher/manager.go`
- Modify: `internal/cluster/manager.go` (hook registry, call sites in `Disconnect` and `healthMonitor`)
- Modify: `internal/services/resource.go` (wire hooks at `ServiceStartup`, around line 69)
- Test: `internal/watcher/manager_test.go`, `internal/cluster/manager_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/watcher/manager_test.go` (virtual sources avoid needing a k8s fake):

```go
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
```

`internal/cluster/manager_test.go` (match the file's existing package form):

```go
func TestManager_DisconnectHook(t *testing.T) {
	m := NewManager(func(string, any) {}, nil, context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	m.SetConnectionForTest("ctx1", &Connection{connCtx: ctx, cancel: cancel})

	var got []string
	m.OnDisconnect(func(name string) { got = append(got, name) })

	testza.AssertNoError(t, m.Disconnect("ctx1"))
	testza.AssertEqual(t, []string{"ctx1"}, got)
}
```

- [ ] **Step 2: Run to verify red** — `go test ./internal/watcher/ ./internal/cluster/ -run 'StopContext|ResyncContext|DisconnectHook' -v` → compile errors (methods don't exist). That counts as red.

- [ ] **Step 3: Implement**

`internal/watcher/manager.go`:

```go
// StopContext synchronously tears down every watch for the given context.
// Returns the keys it stopped so callers can emit follow-up events.
func (m *WatchManager) StopContext(contextName string) []watchKey {
	m.mu.Lock()
	defer m.mu.Unlock()
	var stopped []watchKey
	for key, state := range m.watches {
		if key.contextName != contextName {
			continue
		}
		if state.graceTimer != nil {
			state.graceTimer.Stop()
		}
		if state.stopVirtual != nil {
			state.stopVirtual()
		}
		state.cancel()
		delete(m.watches, key)
		stopped = append(stopped, key)
	}
	return stopped
}

// ResyncContext tears down every watch for the context and asks the frontend
// to re-list + re-watch via the :resync protocol. Used after a connection
// recovers: any watch may be wedged on a dead socket or hold a stale client.
func (m *WatchManager) ResyncContext(contextName string) {
	for _, key := range m.StopContext(contextName) {
		m.emitEvent(fmt.Sprintf("watch:%s:%s:%s:resync", key.contextName, key.gvr, key.namespace), nil)
	}
}
```

`internal/cluster/manager.go` — hook registry on `Manager` (fields `onDisconnect, onRecovery []func(string)`):

```go
// OnDisconnect registers fn to run synchronously whenever a context is
// disconnected (explicit disconnect, re-import replacement).
func (m *Manager) OnDisconnect(fn func(contextName string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDisconnect = append(m.onDisconnect, fn)
}

// OnRecovery registers fn to run whenever a connection transitions from error
// back to connected.
func (m *Manager) OnRecovery(fn func(contextName string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRecovery = append(m.onRecovery, fn)
}

func (m *Manager) runHooks(hooks []func(string), contextName string) {
	for _, fn := range hooks {
		fn(contextName)
	}
}
```

In `Disconnect` (after `conn.cancel()`, before `emitStatus`):

```go
	m.mu.RLock()
	hooks := append([]func(string)(nil), m.onDisconnect...)
	m.mu.RUnlock()
	m.runHooks(hooks, contextName)
```

In `healthMonitor`'s recovery branch (next to the existing `go m.emitDiscovery(ctx, conn.Name)` at manager.go:724):

```go
					m.mu.RLock()
					hooks := append([]func(string)(nil), m.onRecovery...)
					m.mu.RUnlock()
					go m.runHooks(hooks, conn.Name)
```

`internal/services/resource.go` in `ServiceStartup`, right after `s.watchMgr = watcher.NewWatchManager(...)` (line 69):

```go
	cm := s.appService.ClusterManager()
	cm.OnDisconnect(func(name string) { s.watchMgr.StopContext(name) })
	cm.OnRecovery(s.watchMgr.ResyncContext)
```

- [ ] **Step 4: Run to verify green** — `go test ./internal/watcher/ ./internal/cluster/ ./internal/resource/ -timeout 120s` all pass. Then `go build .` (needs CGO; run in background if slow).

- [ ] **Step 5: Commit** — `jj desc -m "feat(cluster): disconnect/recovery hooks tear down and resync watches per context"`

---

### Task 3: Per-request timeouts on health probes and engine List/Get (finding #3)

A half-open TCP socket after sleep currently wedges the 15s health monitor forever — which disables the entire recovery path added in Task 2. No global `rest.Config.Timeout` (it would kill long-lived watch/log streams); per-call `context.WithTimeout` instead.

**Files:**
- Modify: `internal/cluster/manager.go` (`healthMonitor`, `startHealthPoller`)
- Modify: `internal/resource/engine.go` (`List`, `Get`)
- Test: `internal/cluster/health_test.go` (or new `internal/cluster/probe_test.go`)

- [ ] **Step 1: Write the failing test**

Extract the probe into a testable helper first (this is the refactor seam). In the test file:

```go
func TestProbeHealthz_TimesOutOnHangingServer(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hang until test cleanup
	}))
	defer srv.Close()

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	testza.AssertNoError(t, err)
	conn := &Connection{Clientset: cs}

	orig := healthProbeTimeout
	healthProbeTimeout = 100 * time.Millisecond
	defer func() { healthProbeTimeout = orig }()

	done := make(chan error, 1)
	go func() { _, err := probeHealthz(context.Background(), conn); done <- err }()

	select {
	case err := <-done:
		testza.AssertNotNil(t, err) // deadline exceeded, not a hang
	case <-time.After(2 * time.Second):
		t.Fatal("health probe hung on unresponsive server — no timeout applied")
	}
}
```

(Imports: `net/http`, `net/http/httptest`, `k8s.io/client-go/kubernetes`, `k8s.io/client-go/rest`.)

- [ ] **Step 2: Run to verify red** — compile error (`probeHealthz`/`healthProbeTimeout` don't exist). Red.

- [ ] **Step 3: Implement**

`internal/cluster/manager.go`:

```go
// healthProbeTimeout bounds a single /healthz probe. A var so tests can
// shorten it. Without a deadline, a half-open TCP connection after laptop
// sleep wedges the health monitor loop permanently.
var healthProbeTimeout = 10 * time.Second

func probeHealthz(ctx context.Context, conn *Connection) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()
	return conn.Clientset.Discovery().RESTClient().Get().AbsPath("/healthz").Do(probeCtx).Raw()
}
```

In `healthMonitor`, replace the inline probe (line 706) with `body, err := probeHealthz(ctx, conn)`.

In `startHealthPoller`, bound each `CheckHealth` (it makes 5 sequential API calls; give the batch 20s):

```go
func (m *Manager) startHealthPoller(ctx context.Context, contextName string, conn *Connection) {
	check := func() {
		hctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		m.emitEvent(fmt.Sprintf("cluster:%s:health", contextName), CheckHealth(hctx, conn))
	}
	check()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
```

`internal/resource/engine.go` — at the top of both `List` and `Get` (after the virtual-source branch, before `e.client(...)`):

```go
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
```

with a package-level `var requestTimeout = 30 * time.Second` (documented: bounds a single List/Get so a dead socket surfaces as an error instead of a frozen UI).

- [ ] **Step 4: Run to verify green** — `go test ./internal/cluster/ ./internal/resource/ -v -timeout 120s`

- [ ] **Step 5: Commit** — `jj desc -m "fix(cluster): per-request timeouts on health probes and resource List/Get"`

---

### Task 4: Discovery — retry until success, partial-as-error, CRD watch, startup connect retry (findings #4, #5)

**Files:**
- Modify: `internal/cluster/manager.go` (`DiscoverResources`, `emitDiscovery`, delete `discoverWithRetry`, `Activate`)
- Create: `internal/cluster/crd_watch.go`
- Modify: `internal/services/resource.go:142,576`, `internal/services/helm.go:458`, `internal/services/schema.go:43` (tolerate partial results)
- Modify: `internal/services/app.go:83-95` (startup connect retry)
- Test: `internal/cluster/discovery_retry_test.go`, new `internal/cluster/crd_watch_test.go`

- [ ] **Step 1: Write the failing tests**

Replace the `discoverWithRetry` tests in `internal/cluster/discovery_retry_test.go` with tests for the new loop:

```go
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
```

New `internal/cluster/crd_watch_test.go`:

```go
func TestWatchCRDs_TriggersRediscoveryOnCRDChange(t *testing.T) {
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	fw := watch.NewFake()
	defer fw.Stop()
	dyn.PrependWatchReactor("customresourcedefinitions", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, fw, nil
	})

	disc := &discoveryfake.FakeDiscovery{Fake: &k8stesting.Fake{}}
	conn := NewTestConnection(dyn, disc)
	defer conn.CloseForTest()

	discoveryEvents := make(chan struct{}, 8)
	m := NewManager(func(name string, _ any) {
		if name == "discovery:ctx1:resources" {
			discoveryEvents <- struct{}{}
		}
	}, nil, context.Background())
	m.SetConnectionForTest("ctx1", conn)

	origDebounce := crdRediscoverDebounce
	crdRediscoverDebounce = 10 * time.Millisecond
	defer func() { crdRediscoverDebounce = origDebounce }()

	go m.watchCRDs(conn.Context(), "ctx1", conn)
	time.Sleep(50 * time.Millisecond) // let list+watch establish

	crd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "things.xyz.io"},
	}}
	fw.Add(crd)

	select {
	case <-discoveryEvents:
	case <-time.After(2 * time.Second):
		t.Fatal("CRD add did not trigger re-discovery")
	}
}
```

(Imports: `discoveryfake "k8s.io/client-go/discovery/fake"`, `dynamicfake "k8s.io/client-go/dynamic/fake"`, `k8stesting "k8s.io/client-go/testing"`, `"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"`, `"k8s.io/apimachinery/pkg/runtime"`, `"k8s.io/apimachinery/pkg/watch"`.)

- [ ] **Step 2: Run to verify red** — compile errors for `discoverLoop`, `watchCRDs`, `crdRediscoverDebounce`. Red.

- [ ] **Step 3: Implement**

`internal/cluster/manager.go`:

1. `DiscoverResources` — propagate partial-failure. Change line 576-579 comment + the final return:

```go
	lists, err := conn.Discovery.ServerPreferredResources()
	if err != nil && len(lists) == 0 {
		return nil, err
	}
	// err may be non-nil with partial lists (some API groups down). Fall
	// through and return the partial set WITH the error so callers can both
	// use it and know to retry.
```

...and at the end: `return primary, err`.

2. Replace `discoverWithRetry` + `emitDiscovery` with:

```go
// discoverLoop retries discover until it fully succeeds or ctx is cancelled,
// with capped exponential backoff. Partial results (err != nil, len > 0) are
// emitted every round so the UI has something during an outage; a clean
// success emits (even if empty) and ends the loop.
func discoverLoop(ctx context.Context, backoff, maxBackoff time.Duration, discover func() ([]APIResource, error), emit func([]APIResource)) {
	for {
		resources, err := discover()
		if err == nil {
			emit(resources)
			return
		}
		if len(resources) > 0 {
			emit(resources)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
		}
	}
}

func (m *Manager) emitDiscovery(ctx context.Context, contextName string) {
	discoverLoop(ctx, 2*time.Second, time.Minute,
		func() ([]APIResource, error) {
			res, err := m.DiscoverResources(contextName)
			if err != nil {
				slox.Warn(m.ctx, "resource discovery incomplete, will retry", "context", contextName, "error", err)
			}
			return res, err
		},
		func(res []APIResource) {
			m.emitEvent(fmt.Sprintf("discovery:%s:resources", contextName), res)
		})
}
```

3. In `Activate`, add alongside the other goroutines: `go m.watchCRDs(monitorCtx, contextName, conn)`.

New `internal/cluster/crd_watch.go`:

```go
package cluster

import (
	"context"
	"time"

	"github.com/Vilsol/slox"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

// crdRediscoverDebounce coalesces bursts of CRD changes (e.g. installing a
// helm chart with 20 CRDs) into one discovery pass. Var for tests.
var crdRediscoverDebounce = 2 * time.Second

// watchCRDs keeps resource discovery current while a connection stays healthy:
// without it, CRDs installed at runtime only appear after a reconnect.
func (m *Manager) watchCRDs(ctx context.Context, contextName string, conn *Connection) {
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	var debounce *time.Timer
	defer func() {
		if debounce != nil {
			debounce.Stop()
		}
	}()

	for {
		list, err := conn.Dynamic.Resource(crdGVR).List(ctx, metav1.ListOptions{})
		if err != nil {
			if k8serrors.IsForbidden(err) {
				slox.Info(m.ctx, "no permission to watch CRDs, runtime CRD changes won't auto-refresh", "context", contextName)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			continue
		}

		wi, err := conn.Dynamic.Resource(crdGVR).Watch(ctx, metav1.ListOptions{
			ResourceVersion:     list.GetResourceVersion(),
			AllowWatchBookmarks: true,
		})
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for event := range wi.ResultChan() {
			if event.Type == watch.Bookmark || event.Type == watch.Error {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(crdRediscoverDebounce, func() {
				m.emitDiscovery(ctx, contextName)
			})
		}
		wi.Stop()

		select {
		case <-ctx.Done():
			return
		default: // channel closed by server timeout — re-list and re-watch
		}
	}
}
```

4. Partial-tolerant callers — apply this pattern at `internal/services/resource.go:142`, `:576`, `internal/services/helm.go:458`, `internal/services/schema.go:43`:

```go
	discovered, err := s.appService.ClusterManager().DiscoverResources(contextName)
	if err != nil && len(discovered) == 0 {
		return nil, err // (match each site's actual return shape)
	}
	if err != nil {
		slox.Warn(ctx, "using partial discovery results", "context", contextName, "error", err)
	}
```

5. `internal/services/app.go` startup reconnect (replace the goroutine body at lines 83-95). Note: `Connect` makes no network calls, so failures here are config-shaped — a short bounded retry suffices; network-level recovery is owned by the health monitor + discovery loop:

```go
			go func(name string) {
				backoff := 2 * time.Second
				for attempt := 1; ; attempt++ {
					err := a.clusterMgr.Connect(a.ctx, name)
					if err == nil {
						break
					}
					slox.Warn(a.ctx, "startup reconnect failed", "context", name, "attempt", attempt, "error", err)
					if attempt >= 5 {
						return
					}
					select {
					case <-a.ctx.Done():
						return
					case <-time.After(backoff):
					}
					backoff *= 2
				}
				if err := a.clusterMgr.Activate(a.ctx, name); err != nil {
					slox.Warn(a.ctx, "failed to activate cluster on startup", "context", name, "error", err)
				}
				a.portForwardManager.ReconnectSaved(name)
				if a.volumeBrowserSvc != nil {
					a.volumeBrowserSvc.OnClusterConnected(name)
				}
			}(last)
```

- [ ] **Step 4: Run to verify green** — `go test ./internal/cluster/ -v -timeout 120s`, then the full fast set, then `go build .`.

- [ ] **Step 5: Commit** — `jj desc -m "feat(cluster): retry discovery until success, surface partial failures, watch CRDs for runtime changes"`

---

### Task 5: Kubeconfig file watching with credential-hash reconnect (finding #6)

**Files:**
- Create: `internal/cluster/kubeconfig_watch.go`
- Modify: `internal/services/app.go` (start watcher + `reconnectChangedContexts`)
- Modify: `frontend/src/lib/stores/cluster.svelte.ts` (subscribe `kubeconfigs:updated` → `loadContexts`)
- Test: new `internal/cluster/kubeconfig_watch_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestWatchKubeconfigs_DetectsCredentialChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	kubeconfig := func(server string) string {
		return `apiVersion: v1
kind: Config
clusters:
- name: c1
  cluster: {server: ` + server + `}
users:
- name: u1
  user: {}
contexts:
- name: ctx1
  context: {cluster: c1, user: u1}
`
	}
	testza.AssertNoError(t, os.WriteFile(path, []byte(kubeconfig("https://old.example:6443")), 0600))

	updated := make(chan struct{}, 4)
	m := NewManager(func(name string, _ any) {
		if name == "kubeconfigs:updated" {
			updated <- struct{}{}
		}
	}, nil, context.Background())
	testza.AssertNoError(t, m.LoadKubeconfigs([]string{path}))

	origDebounce := kubeconfigDebounce
	kubeconfigDebounce = 20 * time.Millisecond
	defer func() { kubeconfigDebounce = origDebounce }()

	changedCh := make(chan []string, 4)
	testza.AssertNoError(t, m.WatchKubeconfigs(context.Background(), dir,
		func() []string { return []string{path} },
		func(changed []string) { changedCh <- changed }))

	testza.AssertNoError(t, os.WriteFile(path, []byte(kubeconfig("https://new.example:6443")), 0600))

	select {
	case changed := <-changedCh:
		testza.AssertEqual(t, []string{"ctx1"}, changed)
	case <-time.After(3 * time.Second):
		t.Fatal("kubeconfig file change was not detected")
	}
	select {
	case <-updated:
	case <-time.After(time.Second):
		t.Fatal("kubeconfigs:updated event not emitted")
	}
}
```

- [ ] **Step 2: Run to verify red** — compile error (`WatchKubeconfigs`, `kubeconfigDebounce`). Red.

- [ ] **Step 3: Implement**

New `internal/cluster/kubeconfig_watch.go`:

```go
package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Vilsol/slox"
	"github.com/fsnotify/fsnotify"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// kubeconfigDebounce coalesces the write bursts most tools produce when
// rewriting kubeconfigs (truncate+write, atomic rename). Var for tests.
var kubeconfigDebounce = 500 * time.Millisecond

// contextCredHash hashes everything that affects how we connect to the named
// context: the context entry, its cluster (server, CA), and its user (certs,
// tokens, exec config). Changes here mean live connections hold stale creds.
func contextCredHash(cfg *clientcmdapi.Config, name string) string {
	kctx, ok := cfg.Contexts[name]
	if !ok {
		return ""
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(kctx)
	if c, ok := cfg.Clusters[kctx.Cluster]; ok {
		_ = enc.Encode(c)
	}
	if u, ok := cfg.AuthInfos[kctx.AuthInfo]; ok {
		_ = enc.Encode(u)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// WatchKubeconfigs watches every kubeconfig source (default precedence paths,
// current extra paths, and importsDir for klados-managed imports) and reloads
// contexts when any of them change on disk — e.g. a cloud CLI rotating certs.
// onChanged receives the names of contexts whose connection-relevant config
// changed, so the caller can reconnect them.
func (m *Manager) WatchKubeconfigs(ctx context.Context, importsDir string, extraPaths func() []string, onChanged func(changed []string)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	dirs := map[string]struct{}{importsDir: {}}
	for _, p := range append(clientcmd.NewDefaultClientConfigLoadingRules().Precedence, extraPaths()...) {
		dirs[filepath.Dir(p)] = struct{}{}
	}
	for d := range dirs {
		if err := w.Add(d); err != nil {
			slox.Debug(m.ctx, "kubeconfig watch: cannot watch dir", "dir", d, "error", err)
		}
	}

	go func() {
		defer w.Close()
		var debounce *time.Timer
		defer func() {
			if debounce != nil {
				debounce.Stop()
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if !isKubeconfigPath(ev.Name, importsDir, extraPaths()) {
					continue
				}
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(kubeconfigDebounce, func() {
					m.reloadAfterFileChange(extraPaths(), onChanged)
				})
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

func isKubeconfigPath(path, importsDir string, extras []string) bool {
	if filepath.Dir(path) == importsDir {
		return true
	}
	for _, p := range append(clientcmd.NewDefaultClientConfigLoadingRules().Precedence, extras...) {
		if p == path {
			return true
		}
	}
	return false
}

func (m *Manager) reloadAfterFileChange(extras []string, onChanged func([]string)) {
	m.mu.RLock()
	old := m.rawConfig
	m.mu.RUnlock()
	oldHashes := map[string]string{}
	if old != nil {
		for name := range old.Contexts {
			oldHashes[name] = contextCredHash(old, name)
		}
	}

	if err := m.LoadKubeconfigs(extras); err != nil {
		slox.Warn(m.ctx, "kubeconfig reload after file change failed", "error", err)
		return
	}

	m.mu.RLock()
	cur := m.rawConfig
	m.mu.RUnlock()
	var changed []string
	for name := range cur.Contexts {
		if contextCredHash(cur, name) != oldHashes[name] {
			changed = append(changed, name)
		}
	}
	for name := range oldHashes {
		if _, ok := cur.Contexts[name]; !ok {
			changed = append(changed, name)
		}
	}

	slox.Info(m.ctx, "kubeconfig files changed on disk", "changedContexts", changed)
	m.emitEvent("kubeconfigs:updated", m.ListContexts())
	if len(changed) > 0 && onChanged != nil {
		onChanged(changed)
	}
}
```

`internal/services/app.go` — in `ServiceStartup` right after the `LoadKubeconfigs` call (add imports `path/filepath`, `time`, `github.com/adrg/xdg`):

```go
	importsDir := filepath.Join(xdg.ConfigHome, "klados", "kubeconfigs")
	extraPaths := func() []string {
		var p []string
		a.config.Read(func(c *config.Config) { p = append(p, c.KubeconfigPaths...) })
		return p
	}
	if err := a.clusterMgr.WatchKubeconfigs(a.ctx, importsDir, extraPaths, a.reconnectChangedContexts); err != nil {
		slox.Warn(a.ctx, "kubeconfig file watching unavailable", "error", err)
	}
```

(If `config.Config` has no `Read` method, use whatever accessor `ImportKubeconfigContent` uses at `internal/services/cluster.go:234` — it calls `cfg.Read(func(cfg *config.Config){...})`, so `Read` exists.)

New method in `internal/services/app.go` (mirrors the re-import reconnect flow at `cluster.go:292-306`):

```go
// reconnectChangedContexts refreshes live connections whose kubeconfig entry
// changed on disk (cert rotation, server change). Disconnect fires the watch
// teardown hooks; the frontend re-lists on the status events.
func (a *AppService) reconnectChangedContexts(names []string) {
	for _, name := range names {
		if _, err := a.clusterMgr.GetConnection(name); err != nil {
			continue // not connected — nothing to refresh
		}
		activated := a.clusterMgr.IsActivated(name)
		slox.Info(a.ctx, "kubeconfig changed on disk, reconnecting", "context", name)
		if err := a.clusterMgr.Disconnect(name); err != nil {
			slox.Warn(a.ctx, "reconnect: disconnect failed", "context", name, "error", err)
			continue
		}
		if err := a.clusterMgr.Connect(a.ctx, name); err != nil {
			slox.Warn(a.ctx, "reconnect: connect failed", "context", name, "error", err)
			continue
		}
		if activated {
			if err := a.clusterMgr.Activate(a.ctx, name); err != nil {
				slox.Warn(a.ctx, "reconnect: activate failed", "context", name, "error", err)
			}
		}
	}
}
```

`frontend/src/lib/stores/cluster.svelte.ts` — add a field `private kubeconfigsUnsub: (() => void) | null = null;` and at the top of `loadContexts()`:

```ts
    if (!this.kubeconfigsUnsub) {
      this.kubeconfigsUnsub = Events.On("kubeconfigs:updated", () => {
        this.loadContexts();
      });
    }
```

- [ ] **Step 4: Run to verify green** — `go test ./internal/cluster/ -run Kubeconfig -v -timeout 60s`, full fast set, `go build .`, `cd frontend && pnpm check`.

- [ ] **Step 5: Commit** — `jj desc -m "feat(cluster): watch kubeconfig files and reconnect contexts whose credentials changed"`

---

### Task 6: Frontend reconnect handling (findings #7, #8, #9)

Backend Task 2 already resyncs watches on recovery; the frontend must (a) re-list `ResourceStore` when a connection transitions back to connected (covers the disconnect→connect re-import flow where no resync fires), (b) handle the namespace watch's `:resync`, and (c) force-restart the namespace watch on reconnect.

**Files:**
- Modify: `frontend/src/lib/stores/resource.svelte.ts`
- Modify: `frontend/src/lib/stores/cluster.svelte.ts`
- Test: new `frontend/src/lib/__tests__/resource.svelte.test.ts`, extend `frontend/src/lib/__tests__/cluster.svelte.test.ts`

- [ ] **Step 1: Write the failing tests**

New `frontend/src/lib/__tests__/resource.svelte.test.ts`:

```ts
import {describe, it, expect, vi, beforeEach} from "vitest";

vi.mock("../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  ListResourcesWithVersion: vi.fn().mockResolvedValue({items: [], resourceVersion: "1"}),
  StartWatch: vi.fn().mockResolvedValue(undefined),
  StopWatch: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("../../../bindings/github.com/Vilsol/klados/internal/services/appservice.js", () => ({
  LogFrontend: vi.fn().mockResolvedValue(undefined),
}));

import {ListResourcesWithVersion} from "../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice";
import {Events} from "@wailsio/runtime";
import {ResourceStore} from "$lib/stores/resource.svelte";

const mockedList = vi.mocked(ListResourcesWithVersion);
const mockedEventsOn = vi.mocked(Events.On);

function invokeHandler(eventName: string, payload: unknown) {
  const call = mockedEventsOn.mock.calls.find((c) => c[0] === eventName);
  if (!call) throw new Error(`no handler for ${eventName}`);
  (call[1] as (e: unknown) => void)({data: payload});
}

describe("ResourceStore reconnect handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedList.mockResolvedValue({items: [], resourceVersion: "1"} as never);
  });

  it("re-lists when the connection recovers from error", async () => {
    const store = new ResourceStore();
    await store.start("ctx1", "core.v1.pods", "default");
    expect(mockedList).toHaveBeenCalledTimes(1);

    invokeHandler("status:ctx1:connection", "error");
    invokeHandler("status:ctx1:connection", "connected");
    await vi.waitFor(() => expect(mockedList).toHaveBeenCalledTimes(2));

    store.stop();
  });

  it("does not re-list on a connected event without a preceding failure", async () => {
    const store = new ResourceStore();
    await store.start("ctx1", "core.v1.pods", "default");

    invokeHandler("status:ctx1:connection", "connected");
    await new Promise((r) => setTimeout(r, 20));
    expect(mockedList).toHaveBeenCalledTimes(1);

    store.stop();
  });
});
```

(Check how `ResourceStore` is exported — if only page-level instances are created via `new ResourceStore()` from the module, export the class; look at `resource.svelte.ts`'s existing exports and match `ResourceListPage`'s usage.)

Extend `frontend/src/lib/__tests__/cluster.svelte.test.ts`:

```ts
function invokeStatusHandler(ctx: string, status: string) {
  const call = mockedEventsOn.mock.calls.find((c) => c[0] === `status:${ctx}:connection`);
  if (!call) throw new Error(`no status handler for ${ctx}`);
  (call[1] as (e: unknown) => void)({data: status});
}

it("reconnect of the active context restarts the namespace watch", async () => {
  mockedListContexts.mockResolvedValue([
    {name: "ctx1", cluster: "c1", user: "u1", namespace: "default", status: ConnectionStatus.StatusConnected},
  ] as never);
  mockedListNamespaces.mockResolvedValue(["default"] as never);
  await clusterStore.loadContexts(); // registers status handler + auto-restores ctx1

  mockedStartWatch.mockClear();
  mockedStopWatch.mockClear();
  invokeStatusHandler("ctx1", "error");
  invokeStatusHandler("ctx1", "connected");

  await vi.waitFor(() => {
    expect(mockedStopWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "");
    expect(mockedStartWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "", "");
  });
});

it("namespace watch :resync reloads the list and re-watches", async () => {
  mockedListNamespaces.mockResolvedValue(["default"] as never);
  await clusterStore.setActiveContext("ctx1");
  mockedListNamespaces.mockClear();
  mockedStartWatch.mockClear();

  const call = mockedEventsOn.mock.calls.find((c) => c[0] === "watch:ctx1:core.v1.namespaces::resync");
  expect(call).toBeDefined();
  (call![1] as () => void)();

  await vi.waitFor(() => {
    expect(mockedListNamespaces).toHaveBeenCalledWith("ctx1");
    expect(mockedStartWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "", "");
  });
});
```

- [ ] **Step 2: Run to verify red**

Run: `cd frontend && npx vitest run src/lib/__tests__/resource.svelte.test.ts src/lib/__tests__/cluster.svelte.test.ts`
Expected: the new tests FAIL (no status handler registered by ResourceStore; no `:resync` handler; no watch restart on reconnect).

- [ ] **Step 3: Implement**

`frontend/src/lib/stores/resource.svelte.ts` — add fields:

```ts
  private unsubStatus: (() => void) | null = null;
  private lastStatus = "";
```

In `start()`, after the resync subscription (line 54):

```ts
    // Re-list when the backend connection recovers: watches may have died or
    // been replaced (kubeconfig re-import) while this page was open.
    this.unsubStatus = Events.On(`status:${contextName}:connection`, (wailsEvent: unknown) => {
      const status = ((wailsEvent as {data?: unknown})?.data ?? wailsEvent) as string;
      const prev = this.lastStatus;
      this.lastStatus = status;
      // prev !== "" (no status seen yet) keeps the initial page-load connect
      // from re-listing; any other non-connected prev (error, disconnected,
      // connecting — the re-import flow emits disconnected→connecting→connected)
      // must trigger the re-list.
      if (status === "connected" && prev !== "" && prev !== "connected") {
        log.info("connection recovered — re-listing", {contextName, gvr});
        this.loadAndWatch(contextName, gvr, namespace, gen, performance.now()).catch((e) =>
          log.warn("reconnect re-list failed", {contextName, gvr, error: String(e)}),
        );
      }
    });
```

In `stop()`:

```ts
    if (this.unsubStatus) {
      this.unsubStatus();
      this.unsubStatus = null;
    }
    this.lastStatus = "";
```

`frontend/src/lib/stores/cluster.svelte.ts`:

1. `nsWatch` gains a resync unsub: `private nsWatch: {ctx: string; unsub: () => void; unsubResync: () => void} | null = null;`

2. `startNamespaceWatch` — add a `force` parameter and the resync subscription:

```ts
  private async startNamespaceWatch(ctxName: string, force = false) {
    if (!force && this.nsWatch?.ctx === ctxName) return;
    this.stopNamespaceWatch();

    await this.loadNamespaces(ctxName);

    const eventName = `watch:${ctxName}:core.v1.namespaces:`;
    const unsub = Events.On(eventName, (wailsEvent: unknown) => {
      // ... existing handler body unchanged ...
    });
    // Backend emits :resync when the namespace watch hit a 410 and died —
    // reload the list and start a fresh watch or the dropdown goes stale.
    const unsubResync = Events.On(`${eventName}:resync`, () => {
      log.info("namespace watch resync", {ctxName});
      this.loadNamespaces(ctxName);
      StartWatch(ctxName, "core.v1.namespaces", "", "").catch((e) =>
        log.warn("namespace watch restart failed", {ctxName, error: String(e)}),
      );
    });
    this.nsWatch = {ctx: ctxName, unsub, unsubResync};

    StartWatch(ctxName, "core.v1.namespaces", "", "").catch((e) =>
      log.warn("namespace watch start failed", {ctxName, error: String(e)}),
    );
  }
```

3. `stopNamespaceWatch` — call both unsubs:

```ts
  private stopNamespaceWatch() {
    if (!this.nsWatch) return;
    const {ctx, unsub, unsubResync} = this.nsWatch;
    this.nsWatch = null;
    unsub();
    unsubResync();
    StopWatch(ctx, "core.v1.namespaces", "").catch((e) => log.warn("namespace watch stop failed", {ctx, error: String(e)}));
  }
```

4. Status handler in `loadContexts()` (replace lines 168-174's callback body) — force-restart on reconnect of the active context. Force also fixes finding #9 (same context name pointing at a new cluster after re-import):

```ts
        const unsub = Events.On(`status:${ctx.name}:connection`, (wailsEvent: unknown) => {
          const status = ((wailsEvent as {data?: unknown})?.data ?? wailsEvent) as string;
          const prev = this.connectionStatus[ctx.name];
          this.connectionStatus[ctx.name] = (status as ConnectionStatusType) ?? "disconnected";
          if (status !== "connected") return;
          if (!this.activeContext) {
            this.restoreContext(ctx.name);
          } else if (this.activeContext === ctx.name && prev !== "connected") {
            // Recovered or replaced connection: namespaces may have changed and
            // the old watch is gone — restart it unconditionally.
            this.startNamespaceWatch(ctx.name, true);
          }
        });
```

- [ ] **Step 4: Run to verify green**

Run: `cd frontend && pnpm check && pnpm test`
Expected: PASS, including all pre-existing cluster store tests (the "no-op when same context" and switching tests must still pass — the `force` default keeps old behavior).

- [ ] **Step 5: Commit** — `jj desc -m "fix(frontend): re-list resources and restart namespace watch when a connection recovers"`

---

## Final verification (after Task 6)

- [ ] `go test ./internal/... -timeout 300s` (full, CGO — run in background)
- [ ] `cd frontend && pnpm check && pnpm test`
- [ ] `go build .`
- [ ] Update `docs/2026-07-02-liveness-audit.md`: mark findings #1–#9 with a `**Fixed:** <change-id>` line each.
- [ ] Update `.wolf/buglog.json` (one entry per finding fixed, per OpenWolf protocol), `.wolf/anatomy.md` (new files: `crd_watch.go`, `kubeconfig_watch.go`, `resource.svelte.test.ts`), `.wolf/memory.md`.
- [ ] Final jj commit for doc/wolf updates.
