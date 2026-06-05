package services

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/MarvinJWendt/testza"
	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/session"
	"github.com/adrg/xdg"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

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
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`
	testza.AssertNoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func setupClusterService(t *testing.T) (*ClusterService, *session.Session) {
	t.Helper()
	p := writeTestKubeconfig(t)
	t.Setenv("KUBECONFIG", p)

	mgr := cluster.NewManager(func(string, any) {}, nil, context.Background())
	testza.AssertNoError(t, mgr.LoadKubeconfigs(nil))

	sess := &session.Session{
		ConnectedClusters: []string{},
		ActiveNamespaces:  map[string]string{},
	}

	appSvc := &AppService{clusterMgr: mgr}
	svc := &ClusterService{
		appService: appSvc,
		session:    sess,
	}

	return svc, sess
}

func TestClusterService_ListContexts(t *testing.T) {
	svc, _ := setupClusterService(t)

	contexts := svc.ListContexts()
	testza.AssertTrue(t, len(contexts) > 0)
	testza.AssertEqual(t, "test-context", contexts[0].Name)
}

func TestClusterService_SwitchNamespace(t *testing.T) {
	svc, sess := setupClusterService(t)

	testza.AssertNoError(t, svc.SwitchNamespace("test-context", "kube-system"))

	testza.AssertEqual(t, "kube-system", sess.ActiveNamespaces["test-context"])
}

func TestClusterService_GetStatusDisconnected(t *testing.T) {
	svc, _ := setupClusterService(t)

	status := svc.GetStatus("test-context")
	testza.AssertEqual(t, cluster.StatusDisconnected, status)
}

func TestClusterService_DisconnectUpdatesSession(t *testing.T) {
	svc, sess := setupClusterService(t)

	sess.ConnectedClusters = []string{"test-context", "other"}

	testza.AssertNoError(t, svc.Disconnect("test-context"))

	testza.AssertEqual(t, []string{"other"}, sess.ConnectedClusters)
}

func TestAppendUnique(t *testing.T) {
	result := appendUnique([]string{"a", "b"}, "b")
	testza.AssertEqual(t, []string{"a", "b"}, result)

	result = appendUnique([]string{"a", "b"}, "c")
	testza.AssertEqual(t, []string{"a", "b", "c"}, result)
}

func TestRemoveString(t *testing.T) {
	result := removeString([]string{"a", "b", "c"}, "b")
	testza.AssertEqual(t, []string{"a", "c"}, result)

	result = removeString([]string{"a"}, "x")
	testza.AssertEqual(t, []string{"a"}, result)
}

func writeSecondKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config2")
	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.2:6443
  name: second-cluster
contexts:
- context:
    cluster: second-cluster
    user: second-user
  name: second-context
current-context: second-context
users:
- name: second-user
  user:
    token: fake-token-2
`
	testza.AssertNoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func setupClusterServiceWithConfig(t *testing.T) (*ClusterService, *config.Config) {
	t.Helper()
	p := writeTestKubeconfig(t)
	t.Setenv("KUBECONFIG", p)

	mgr := cluster.NewManager(func(string, any) {}, nil, context.Background())
	testza.AssertNoError(t, mgr.LoadKubeconfigs(nil))

	sess := &session.Session{
		ConnectedClusters: []string{},
		ActiveNamespaces:  map[string]string{},
	}

	cfg := config.DefaultConfig()
	appSvc := &AppService{clusterMgr: mgr, config: cfg}
	svc := &ClusterService{
		appService: appSvc,
		session:    sess,
		ctx:        context.Background(),
	}

	return svc, cfg
}

func TestRemoveKubeconfigPath_RemovesAndReloads(t *testing.T) {
	svc, cfg := setupClusterServiceWithConfig(t)

	second := writeSecondKubeconfig(t)
	_, err := svc.AddKubeconfigPath(second)
	testza.AssertNoError(t, err)

	contexts, err := svc.RemoveKubeconfigPath(second)
	testza.AssertNoError(t, err)

	for _, p := range cfg.KubeconfigPaths {
		testza.AssertNotEqual(t, second, p)
	}
	for _, kc := range contexts {
		testza.AssertNotEqual(t, "second-context", kc.Name)
	}
}

func TestRemoveKubeconfigPath_NoOpWhenAbsent(t *testing.T) {
	svc, cfg := setupClusterServiceWithConfig(t)

	before := len(cfg.KubeconfigPaths)
	contexts, err := svc.RemoveKubeconfigPath("/nonexistent/path/config")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, before, len(cfg.KubeconfigPaths))
	testza.AssertTrue(t, len(contexts) > 0)
}

func TestRemoveKubeconfigPath_RejectsDefault(t *testing.T) {
	svc, _ := setupClusterServiceWithConfig(t)

	defaultPath := writeTestKubeconfig(t)
	t.Setenv("KUBECONFIG", defaultPath)

	_, err := svc.RemoveKubeconfigPath(defaultPath)
	testza.AssertNotNil(t, err)
	testza.AssertContains(t, err.Error(), "cannot forget default kubeconfig path")
}

func TestRemoveKubeconfigPath_PrunesClusterPrefs(t *testing.T) {
	svc, cfg := setupClusterServiceWithConfig(t)

	second := writeSecondKubeconfig(t)
	_, err := svc.AddKubeconfigPath(second)
	testza.AssertNoError(t, err)

	testza.AssertNoError(t, cfg.Update(func(c *config.Config) {
		if c.Clusters == nil {
			c.Clusters = make(map[string]*config.ClusterPrefs)
		}
		c.Clusters["second-context"] = &config.ClusterPrefs{}
	}))

	_, err = svc.RemoveKubeconfigPath(second)
	testza.AssertNoError(t, err)

	cfg.Read(func(c *config.Config) {
		_, exists := c.Clusters["second-context"]
		testza.AssertFalse(t, exists)
	})
}

func TestSupersedes(t *testing.T) {
	newC := map[string]struct{}{"a": {}, "b": {}}
	testza.AssertTrue(t, supersedes(newC, []string{"a"}))
	testza.AssertTrue(t, supersedes(newC, []string{"a", "b"}))
	testza.AssertFalse(t, supersedes(newC, []string{"a", "c"}))
	testza.AssertFalse(t, supersedes(newC, []string{}))
}

func importableKubeconfig(ctxName, clusterName, server string) string {
	return `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: ` + server + `
  name: ` + clusterName + `
contexts:
- context:
    cluster: ` + clusterName + `
    user: ` + ctxName + `-user
  name: ` + ctxName + `
users:
- name: ` + ctxName + `-user
  user:
    token: tok
`
}

func countImportedPaths(paths []string, dir string) int {
	n := 0
	for _, p := range paths {
		if filepath.Dir(p) == dir {
			n++
		}
	}
	return n
}

func TestImportKubeconfigContent_ReplacesSupersededImport(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	xdg.Reload()
	importDir := filepath.Join(xdg.ConfigHome, "klados", "kubeconfigs")

	svc, cfg := setupClusterServiceWithConfig(t)

	v1 := importableKubeconfig("imported-ctx", "cluster-v1", "https://127.0.0.1:6443")
	_, err := svc.ImportKubeconfigContent(v1)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, 1, countImportedPaths(cfg.KubeconfigPaths, importDir))

	// Re-import the same context with updated server — must replace, not duplicate.
	v2 := importableKubeconfig("imported-ctx", "cluster-v2", "https://127.0.0.2:6443")
	_, err = svc.ImportKubeconfigContent(v2)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, 1, countImportedPaths(cfg.KubeconfigPaths, importDir))

	info, err := svc.GetClusterInfo("imported-ctx")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "https://127.0.0.2:6443", info.ServerURL)
}

func TestImportKubeconfigContent_ReconnectsConnectedContext(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	xdg.Reload()

	svc, _ := setupClusterServiceWithConfig(t)

	v1 := importableKubeconfig("imported-ctx", "cluster-v1", "https://127.0.0.1:6443")
	_, err := svc.ImportKubeconfigContent(v1)
	testza.AssertNoError(t, err)

	testza.AssertNoError(t, svc.manager().Connect(context.Background(), "imported-ctx"))
	conn, err := svc.manager().GetConnection("imported-ctx")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "https://127.0.0.1:6443", conn.Config.Host)

	v2 := importableKubeconfig("imported-ctx", "cluster-v2", "https://127.0.0.2:6443")
	_, err = svc.ImportKubeconfigContent(v2)
	testza.AssertNoError(t, err)

	conn2, err := svc.manager().GetConnection("imported-ctx")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "https://127.0.0.2:6443", conn2.Config.Host)
}

func TestGetClusterInfo_Disconnected(t *testing.T) {
	svc, _ := setupClusterServiceWithConfig(t)

	info, err := svc.GetClusterInfo("test-context")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "test-context", info.Context.Name)
	testza.AssertEqual(t, "https://127.0.0.1:6443", info.ServerURL)
	testza.AssertEqual(t, CapabilityUnknown, info.MetricsServer)
	testza.AssertEqual(t, "", info.PrometheusURL)
}

func TestGetClusterInfo_ConfiguredPrometheus(t *testing.T) {
	svc, cfg := setupClusterServiceWithConfig(t)

	testza.AssertNoError(t, cfg.Update(func(c *config.Config) {
		if c.Metrics == nil {
			c.Metrics = make(map[string]*config.MetricsConfig)
		}
		c.Metrics["test-context"] = &config.MetricsConfig{PrometheusURL: "http://prometheus.example.com"}
	}))

	info, err := svc.GetClusterInfo("test-context")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "http://prometheus.example.com", info.PrometheusURL)
	testza.AssertEqual(t, "configured", info.PrometheusSource)
}
