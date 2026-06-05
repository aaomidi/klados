package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/metrics"
	"github.com/Vilsol/klados/internal/session"
	"github.com/Vilsol/slox"
	"github.com/adrg/xdg"
	"github.com/wailsapp/wails/v3/pkg/application"
	"k8s.io/client-go/tools/clientcmd"
)

type ClusterService struct {
	appService       *AppService
	session          *session.Session
	volumeBrowserSvc *VolumeBrowserService
	helmSvc          *HelmService
	ctx              context.Context
}

func NewClusterService(appSvc *AppService, sess *session.Session) *ClusterService {
	return &ClusterService{
		appService: appSvc,
		session:    sess,
	}
}

//wails:ignore
func (c *ClusterService) SetVolumeBrowserService(svc *VolumeBrowserService) {
	c.volumeBrowserSvc = svc
}

//wails:ignore
func (c *ClusterService) SetHelmService(svc *HelmService) {
	c.helmSvc = svc
}

func (c *ClusterService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	c.ctx = ctx
	return nil
}

func (c *ClusterService) manager() *cluster.Manager {
	return c.appService.ClusterManager()
}

func (c *ClusterService) ListContexts() []cluster.KubeContext {
	return c.manager().ListContexts()
}

func (c *ClusterService) Connect(contextName string) error {
	if err := c.manager().Connect(c.ctx, contextName); err != nil {
		return err
	}

	c.session.ConnectedClusters = appendUnique(c.session.ConnectedClusters, contextName)
	c.session.SaveDebounced()

	if ps := c.appService.PluginService(); ps != nil {
		ps.EmitClusterEvent("cluster:connected", clusterEventPayload(contextName))
	}

	go c.appService.PortForwardManager().ReconnectSaved(contextName)

	if c.volumeBrowserSvc != nil {
		c.volumeBrowserSvc.OnClusterConnected(contextName)
	}

	if c.helmSvc != nil {
		go c.helmSvc.OnClusterConnected(c.ctx, contextName)
	}

	return nil
}

func (c *ClusterService) Disconnect(contextName string) error {
	if err := c.manager().Disconnect(contextName); err != nil {
		return err
	}

	c.session.ConnectedClusters = removeString(c.session.ConnectedClusters, contextName)
	c.session.SaveDebounced()

	if vbm := c.appService.VolumeBrowserManager(); vbm != nil {
		if err := vbm.StopForContext(c.ctx, contextName); err != nil {
			slox.Warn(c.ctx, "volumebrowser: cleanup on disconnect failed", "context", contextName, "error", err)
		}
	}

	if ps := c.appService.PluginService(); ps != nil {
		ps.EmitClusterEvent("cluster:disconnected", clusterEventPayload(contextName))
	}
	return nil
}

func (c *ClusterService) Activate(contextName string) error {
	if err := c.manager().Activate(c.ctx, contextName); err != nil {
		return err
	}
	if ps := c.appService.PluginService(); ps != nil {
		ps.EmitClusterEvent("cluster:activated", clusterEventPayload(contextName))
	}
	return nil
}

func (c *ClusterService) Deactivate(contextName string) error {
	c.manager().Deactivate(contextName)
	if ps := c.appService.PluginService(); ps != nil {
		ps.EmitClusterEvent("cluster:deactivated", clusterEventPayload(contextName))
	}
	return nil
}

func (c *ClusterService) ListNamespaces(contextName string) ([]string, error) {
	return c.manager().ListNamespaces(c.ctx, contextName)
}

func (c *ClusterService) SwitchNamespace(contextName, namespace string) error {
	if c.session.ActiveNamespaces == nil {
		c.session.ActiveNamespaces = make(map[string]string)
	}
	c.session.ActiveNamespaces[contextName] = namespace
	c.session.SaveDebounced()

	if ps := c.appService.PluginService(); ps != nil {
		payload, _ := json.Marshal(map[string]string{"context": contextName, "namespace": namespace})
		ps.EmitClusterEvent("namespace:changed", payload)
	}
	return nil
}

func (c *ClusterService) GetActiveNamespace(contextName string) string {
	if c.session.ActiveNamespaces != nil {
		if ns, ok := c.session.ActiveNamespaces[contextName]; ok {
			return ns
		}
	}
	return ""
}

func (c *ClusterService) GetStatus(contextName string) cluster.ConnectionStatus {
	conn, err := c.manager().GetConnection(contextName)
	if err != nil {
		return cluster.StatusDisconnected
	}
	return conn.Status
}

func (c *ClusterService) CreateNamespace(contextName, name string) error {
	return c.manager().CreateNamespace(c.ctx, contextName, name)
}

func (c *ClusterService) DeleteNamespace(contextName, name string) error {
	return c.manager().DeleteNamespace(c.ctx, contextName, name)
}

func (c *ClusterService) AddKubeconfigPath(path string) ([]cluster.KubeContext, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("cannot read file: %w", err)
	}
	cfg := c.appService.Config()
	if err := cfg.Update(func(cfg *config.Config) {
		for _, p := range cfg.KubeconfigPaths {
			if p == path {
				return
			}
		}
		cfg.KubeconfigPaths = append(cfg.KubeconfigPaths, path)
	}); err != nil {
		return nil, err
	}
	if err := c.manager().LoadKubeconfigs(cfg.KubeconfigPaths); err != nil {
		return nil, err
	}
	return c.manager().ListContexts(), nil
}

// supersedes reports whether a new import defining newCtxs fully replaces an
// existing klados-managed kubeconfig defining existingCtxs — i.e. every context
// in the existing file is also present in the new import. An empty existing set
// never qualifies.
func supersedes(newCtxs map[string]struct{}, existingCtxs []string) bool {
	if len(existingCtxs) == 0 {
		return false
	}
	for _, name := range existingCtxs {
		if _, ok := newCtxs[name]; !ok {
			return false
		}
	}
	return true
}

func contextNamesFromFile(path string) ([]string, error) {
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	return names, nil
}

func (c *ClusterService) ImportKubeconfigContent(content string) ([]cluster.KubeContext, error) {
	parsed, err := clientcmd.Load([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("invalid kubeconfig: %w", err)
	}
	newCtxs := make(map[string]struct{}, len(parsed.Contexts))
	for name := range parsed.Contexts {
		newCtxs[name] = struct{}{}
	}

	dir := filepath.Join(xdg.ConfigHome, "klados", "kubeconfigs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	// Find prior klados-managed imports this one fully supersedes, so an updated
	// kubeconfig replaces the stale copy instead of being shadowed by it during
	// the clientcmd merge.
	cfg := c.appService.Config()
	var stalePaths []string
	cfg.Read(func(cfg *config.Config) {
		for _, p := range cfg.KubeconfigPaths {
			if filepath.Dir(p) != dir {
				continue
			}
			existing, err := contextNamesFromFile(p)
			if err != nil {
				continue
			}
			if supersedes(newCtxs, existing) {
				stalePaths = append(stalePaths, p)
			}
		}
	})

	sum := sha256.Sum256([]byte(content))
	path := filepath.Join(dir, fmt.Sprintf("%x", sum[:4])+".yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return nil, err
	}

	if len(stalePaths) > 0 {
		if err := cfg.Update(func(cfg *config.Config) {
			kept := make([]string, 0, len(cfg.KubeconfigPaths))
			for _, p := range cfg.KubeconfigPaths {
				if !contains(stalePaths, p) {
					kept = append(kept, p)
				}
			}
			cfg.KubeconfigPaths = kept
		}); err != nil {
			return nil, err
		}
		for _, sp := range stalePaths {
			if sp != path {
				if err := os.Remove(sp); err != nil && !os.IsNotExist(err) {
					slox.Warn(c.ctx, "import: removing superseded kubeconfig failed", "path", sp, "error", err)
				}
			}
		}
	}

	// Snapshot which imported contexts are currently connected (and whether they
	// were activated) so we can reconnect them with the refreshed credentials.
	reactivate := map[string]bool{}
	var reconnect []string
	for name := range newCtxs {
		if _, err := c.manager().GetConnection(name); err == nil {
			reconnect = append(reconnect, name)
			reactivate[name] = c.manager().IsActivated(name)
		}
	}

	contexts, err := c.AddKubeconfigPath(path)
	if err != nil {
		return nil, err
	}

	for _, name := range reconnect {
		if err := c.manager().Disconnect(name); err != nil {
			slox.Warn(c.ctx, "import: reconnect disconnect failed", "context", name, "error", err)
			continue
		}
		if err := c.manager().Connect(c.ctx, name); err != nil {
			slox.Warn(c.ctx, "import: reconnect connect failed", "context", name, "error", err)
			continue
		}
		if reactivate[name] {
			if err := c.manager().Activate(c.ctx, name); err != nil {
				slox.Warn(c.ctx, "import: reconnect activate failed", "context", name, "error", err)
			}
		}
	}
	if len(reconnect) > 0 {
		contexts = c.manager().ListContexts()
	}

	return contexts, nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (c *ClusterService) RemoveKubeconfigPath(path string) ([]cluster.KubeContext, error) {
	for _, p := range clientcmd.NewDefaultClientConfigLoadingRules().Precedence {
		if p == path {
			return nil, fmt.Errorf("cannot forget default kubeconfig path")
		}
	}

	cfg := c.appService.Config()
	if err := cfg.Update(func(cfg *config.Config) {
		filtered := cfg.KubeconfigPaths[:0]
		for _, p := range cfg.KubeconfigPaths {
			if p != path {
				filtered = append(filtered, p)
			}
		}
		cfg.KubeconfigPaths = append([]string(nil), filtered...)
	}); err != nil {
		return nil, err
	}

	if err := c.manager().LoadKubeconfigs(cfg.KubeconfigPaths); err != nil {
		return nil, err
	}

	newContexts := c.manager().ListContexts()
	alive := make(map[string]struct{}, len(newContexts))
	for _, kc := range newContexts {
		alive[kc.Name] = struct{}{}
	}
	_ = cfg.Update(func(cfg *config.Config) {
		for name := range cfg.Clusters {
			if _, ok := alive[name]; !ok {
				delete(cfg.Clusters, name)
			}
		}
	})

	return newContexts, nil
}

type CapabilityState string

const (
	CapabilityAvailable   CapabilityState = "available"
	CapabilityUnavailable CapabilityState = "unavailable"
	CapabilityUnknown     CapabilityState = "unknown"
)

type ClusterInfo struct {
	Context          cluster.KubeContext `json:"context"`
	ServerURL        string              `json:"serverUrl"`
	MetricsServer    CapabilityState     `json:"metricsServer"`
	PrometheusURL    string              `json:"prometheusUrl"`
	PrometheusSource string              `json:"prometheusSource"`
}

func (c *ClusterService) GetClusterInfo(ctxName string) (ClusterInfo, error) {
	info := ClusterInfo{MetricsServer: CapabilityUnknown}

	var found *cluster.KubeContext
	for _, kc := range c.manager().ListContexts() {
		if kc.Name == ctxName {
			kcCopy := kc
			found = &kcCopy
			break
		}
	}
	if found == nil {
		return info, fmt.Errorf("context not found: %s", ctxName)
	}
	info.Context = *found

	if raw := c.manager().RawConfig(); raw != nil {
		if kctx, ok := raw.Contexts[ctxName]; ok {
			if clst, ok := raw.Clusters[kctx.Cluster]; ok {
				info.ServerURL = clst.Server
			}
		}
	}

	resolved := c.appService.Config().ResolveForCluster(ctxName)
	if resolved.Metrics != nil && resolved.Metrics.PrometheusURL != "" {
		info.PrometheusURL = resolved.Metrics.PrometheusURL
		info.PrometheusSource = "configured"
	}

	conn, err := c.manager().GetConnection(ctxName)
	if err == nil && conn.Status == cluster.StatusConnected {
		cap := conn.MetricsCapability
		if cap.HasMetricsServer {
			info.MetricsServer = CapabilityAvailable
		} else {
			info.MetricsServer = CapabilityUnavailable
		}
		if info.PrometheusURL == "" {
			if url, found := metrics.DetectPrometheus(c.ctx, conn.Clientset, conn.Discovery, conn.Dynamic, conn.Config, ""); found {
				info.PrometheusURL = url
				info.PrometheusSource = "detected"
			}
		}
	}
	return info, nil
}

func clusterEventPayload(contextName string) []byte {
	b, _ := json.Marshal(map[string]string{"context": contextName})
	return b
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
