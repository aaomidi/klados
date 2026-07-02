package cluster

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func writeTestKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config")

	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
    namespace: default
  name: test-context
- context:
    cluster: test-cluster
    user: test-user
    namespace: kube-system
  name: test-context-2
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`
	testza.AssertNoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestLoadKubeconfigs(t *testing.T) {
	p := writeTestKubeconfig(t)
	t.Setenv("KUBECONFIG", p)

	var events []string
	var mu sync.Mutex
	emit := func(name string, data any) {
		mu.Lock()
		events = append(events, name)
		mu.Unlock()
	}

	mgr := NewManager(emit, nil, noopLogger())
	testza.AssertNoError(t, mgr.LoadKubeconfigs(nil))

	contexts := mgr.ListContexts()
	testza.AssertTrue(t, len(contexts) >= 2)

	names := make(map[string]bool)
	for _, c := range contexts {
		names[c.Name] = true
	}
	testza.AssertTrue(t, names["test-context"])
	testza.AssertTrue(t, names["test-context-2"])
}

func TestLoadKubeconfigs_ExtraPaths(t *testing.T) {
	p := writeTestKubeconfig(t)
	t.Setenv("KUBECONFIG", "")

	mgr := NewManager(nil, nil, noopLogger())
	testza.AssertNoError(t, mgr.LoadKubeconfigs([]string{p}))

	contexts := mgr.ListContexts()
	testza.AssertTrue(t, len(contexts) >= 2)
}

func writeExtraKubeconfig(t *testing.T, contextName string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "extra-config")
	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:9999
  name: extra-cluster
contexts:
- context:
    cluster: extra-cluster
    user: extra-user
  name: ` + contextName + `
users:
- name: extra-user
  user:
    token: extra-token
`
	testza.AssertNoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestLoadKubeconfigs_SourcePath_ExtraPath(t *testing.T) {
	extraPath := writeExtraKubeconfig(t, "extra-context")
	t.Setenv("KUBECONFIG", "")

	mgr := NewManager(nil, nil, noopLogger())
	testza.AssertNoError(t, mgr.LoadKubeconfigs([]string{extraPath}))

	contexts := mgr.ListContexts()
	var found *KubeContext
	for i := range contexts {
		if contexts[i].Name == "extra-context" {
			found = &contexts[i]
			break
		}
	}
	testza.AssertNotNil(t, found)
	testza.AssertEqual(t, extraPath, found.SourcePath)
	testza.AssertFalse(t, found.IsDefault)
}

func TestLoadKubeconfigs_SourcePath_DefaultIsDefault(t *testing.T) {
	p := writeTestKubeconfig(t)
	t.Setenv("KUBECONFIG", p)

	mgr := NewManager(nil, nil, noopLogger())
	testza.AssertNoError(t, mgr.LoadKubeconfigs(nil))

	contexts := mgr.ListContexts()
	var found *KubeContext
	for i := range contexts {
		if contexts[i].Name == "test-context" {
			found = &contexts[i]
			break
		}
	}
	testza.AssertNotNil(t, found)
	testza.AssertEqual(t, p, found.SourcePath)
	testza.AssertTrue(t, found.IsDefault)
}

func TestLoadKubeconfigs_SourcePath_Precedence(t *testing.T) {
	// same context name in two files — earlier file wins
	first := writeExtraKubeconfig(t, "shared-context")
	second := writeExtraKubeconfig(t, "shared-context")
	t.Setenv("KUBECONFIG", "")

	mgr := NewManager(nil, nil, noopLogger())
	testza.AssertNoError(t, mgr.LoadKubeconfigs([]string{first, second}))

	contexts := mgr.ListContexts()
	var found *KubeContext
	for i := range contexts {
		if contexts[i].Name == "shared-context" {
			found = &contexts[i]
			break
		}
	}
	testza.AssertNotNil(t, found)
	testza.AssertEqual(t, first, found.SourcePath)
}

func TestDisconnectNonexistent(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	err := mgr.Disconnect("test-context")
	testza.AssertNoError(t, err)
}

func TestGetConnectionNotConnected(t *testing.T) {
	mgr := NewManager(nil, nil, noopLogger())
	_, err := mgr.GetConnection("nonexistent")
	testza.AssertNotNil(t, err)
}

func TestDisconnectAllClearsConnections(t *testing.T) {
	var events []string
	var mu sync.Mutex
	emit := func(name string, _ any) {
		mu.Lock()
		events = append(events, name)
		mu.Unlock()
	}

	mgr := NewManager(emit, nil, noopLogger())
	// Inject fake connections directly (same package — private field accessible)
	ctx1cancel := func() {}
	ctx2cancel := func() {}
	mgr.connections["ctx1"] = &Connection{
		KubeContext: KubeContext{Name: "ctx1", Status: StatusConnected},
		cancel:      ctx1cancel,
	}
	mgr.connections["ctx2"] = &Connection{
		KubeContext: KubeContext{Name: "ctx2", Status: StatusConnected},
		cancel:      ctx2cancel,
	}

	testza.AssertNoError(t, mgr.DisconnectAll())
	testza.AssertEqual(t, 0, len(mgr.connections))

	_, err1 := mgr.GetConnection("ctx1")
	testza.AssertNotNil(t, err1)
	_, err2 := mgr.GetConnection("ctx2")
	testza.AssertNotNil(t, err2)
}

func TestIndependentConnectionsIsolated(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	mgr.connections["ctx1"] = &Connection{
		KubeContext: KubeContext{Name: "ctx1", Status: StatusConnected},
		cancel:      func() {},
	}
	mgr.connections["ctx2"] = &Connection{
		KubeContext: KubeContext{Name: "ctx2", Status: StatusConnected},
		cancel:      func() {},
	}

	// Disconnecting ctx1 must not affect ctx2
	testza.AssertNoError(t, mgr.Disconnect("ctx1"))

	_, err := mgr.GetConnection("ctx1")
	testza.AssertNotNil(t, err)

	conn, err := mgr.GetConnection("ctx2")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "ctx2", conn.Name)
}

func TestActivate_ErrorsWhenNotConnected(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	err := mgr.Activate(context.Background(), "nonexistent")
	testza.AssertNotNil(t, err)
}

func TestActivate_Idempotent(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	cancelCalled := 0
	mgr.connections["ctx1"] = &Connection{
		KubeContext:   KubeContext{Name: "ctx1", Status: StatusConnected},
		cancel:        func() {},
		connCtx:       context.Background(),
		activated:     true,
		monitorCancel: func() { cancelCalled++ },
	}

	// Already activated — should be a no-op, not spawn goroutines or overwrite monitorCancel.
	err := mgr.Activate(context.Background(), "ctx1")
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, mgr.connections["ctx1"].activated)
	testza.AssertEqual(t, 0, cancelCalled)
}

func TestDeactivate_CancelsMonitorAndClearsFlag(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	cancelCalled := 0
	mgr.connections["ctx1"] = &Connection{
		KubeContext:   KubeContext{Name: "ctx1", Status: StatusConnected},
		cancel:        func() {},
		connCtx:       context.Background(),
		activated:     true,
		monitorCancel: func() { cancelCalled++ },
	}

	mgr.Deactivate("ctx1")

	testza.AssertEqual(t, 1, cancelCalled)
	testza.AssertFalse(t, mgr.connections["ctx1"].activated)
	testza.AssertTrue(t, mgr.connections["ctx1"].monitorCancel == nil)

	// Second call is a no-op — cancelCalled stays at 1.
	mgr.Deactivate("ctx1")
	testza.AssertEqual(t, 1, cancelCalled)
}

func TestDeactivate_NoopWhenNotActivated(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	mgr.connections["ctx1"] = &Connection{
		KubeContext: KubeContext{Name: "ctx1", Status: StatusConnected},
		cancel:      func() {},
		connCtx:     context.Background(),
	}

	// Should not panic even though monitorCancel is nil.
	mgr.Deactivate("ctx1")
	mgr.Deactivate("nonexistent")
}

func TestDisconnect_DeactivatesFirst(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	monitorCancelCalled := 0
	connCancelCalled := 0
	mgr.connections["ctx1"] = &Connection{
		KubeContext:   KubeContext{Name: "ctx1", Status: StatusConnected},
		cancel:        func() { connCancelCalled++ },
		connCtx:       context.Background(),
		activated:     true,
		monitorCancel: func() { monitorCancelCalled++ },
	}

	testza.AssertNoError(t, mgr.Disconnect("ctx1"))

	testza.AssertEqual(t, 1, monitorCancelCalled)
	testza.AssertEqual(t, 1, connCancelCalled)
	_, err := mgr.GetConnection("ctx1")
	testza.AssertNotNil(t, err)
}

func TestIsActivated(t *testing.T) {
	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	testza.AssertFalse(t, mgr.IsActivated("ctx1"))

	mgr.connections["ctx1"] = &Connection{
		KubeContext: KubeContext{Name: "ctx1", Status: StatusConnected},
		cancel:      func() {},
		connCtx:     context.Background(),
	}
	testza.AssertFalse(t, mgr.IsActivated("ctx1"))

	mgr.connections["ctx1"].activated = true
	testza.AssertTrue(t, mgr.IsActivated("ctx1"))
}

func TestConcurrentAccess(t *testing.T) {
	p := writeTestKubeconfig(t)
	t.Setenv("KUBECONFIG", p)

	mgr := NewManager(func(string, any) {}, nil, noopLogger())
	testza.AssertNoError(t, mgr.LoadKubeconfigs(nil))

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.ListContexts()
		}()
	}
	wg.Wait()
}

func TestManager_DisconnectHook(t *testing.T) {
	m := NewManager(func(string, any) {}, nil, context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	m.SetConnectionForTest("ctx1", &Connection{connCtx: ctx, cancel: cancel})

	var got []string
	m.OnDisconnect(func(name string) { got = append(got, name) })

	testza.AssertNoError(t, m.Disconnect("ctx1"))
	testza.AssertEqual(t, []string{"ctx1"}, got)
}
