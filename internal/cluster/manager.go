package cluster

import (
	"context"
	"fmt"
	"strings"
	"github.com/sasha-s/go-deadlock"
	"time"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/metrics"
	"github.com/Vilsol/slox"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	metricsClientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

type ConnectionStatus int

const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
	StatusError
)

func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "disconnected"
	case StatusConnecting:
		return "connecting"
	case StatusConnected:
		return "connected"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

type KubeContext struct {
	Name          string           `json:"name"`
	Cluster       string           `json:"cluster"`
	User          string           `json:"user"`
	Namespace     string           `json:"namespace"`
	Status        ConnectionStatus `json:"status"`
	ServerVersion string           `json:"serverVersion"`
	Provider      string           `json:"provider"`
	SourcePath    string           `json:"sourcePath"`
	IsDefault     bool             `json:"isDefault"`
}

func detectProvider(clusterName, serverVersion string) string {
	switch {
	case strings.HasPrefix(clusterName, "gke_"):
		return "GKE"
	case strings.HasPrefix(clusterName, "arn:aws:eks"):
		return "EKS"
	case strings.Contains(clusterName, "minikube"):
		return "minikube"
	case strings.Contains(clusterName, "kind-"):
		return "kind"
	case strings.Contains(serverVersion, "k3s"):
		return "k3s"
	default:
		return ""
	}
}

type Connection struct {
	KubeContext
	Config            *rest.Config
	Clientset         kubernetes.Interface
	Dynamic           dynamic.Interface
	Discovery         discovery.DiscoveryInterface
	MetricsCapability metrics.MetricsCapability
	Permissions       PermissionSet
	cancel            context.CancelFunc
	connCtx           context.Context
	monitorCancel     context.CancelFunc
	activated         bool
}

// Context returns the connection-scoped context, cancelled on Disconnect.
// Anything whose lifetime should not outlive the connection (watches, streams)
// must derive from it.
func (c *Connection) Context() context.Context {
	return c.connCtx
}

type Manager struct {
	mu                  deadlock.RWMutex
	connections         map[string]*Connection
	contexts            []KubeContext
	rawConfig           *clientcmdapi.Config
	emitEvent           func(string, any)
	config              *config.Config
	ctx                 context.Context
	discoveredResources map[string][]APIResource
	onDisconnect        []func(string)
	onRecovery          []func(string)
}

func NewManager(emitEvent func(string, any), cfg *config.Config, ctx context.Context) *Manager {
	return &Manager{
		connections:         make(map[string]*Connection),
		emitEvent:           emitEvent,
		config:              cfg,
		ctx:                 ctx,
		discoveredResources: map[string][]APIResource{},
	}
}

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

type sourceEntry struct {
	path      string
	isDefault bool
}

func (m *Manager) LoadKubeconfigs(extraPaths []string) error {
	defaultPaths := clientcmd.NewDefaultClientConfigLoadingRules().Precedence

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if len(extraPaths) > 0 {
		rules.Precedence = append(rules.Precedence, extraPaths...)
	}

	cfg, err := rules.Load()
	if err != nil {
		return fmt.Errorf("loading kubeconfigs: %w", err)
	}

	defaultSet := make(map[string]bool, len(defaultPaths))
	for _, p := range defaultPaths {
		defaultSet[p] = true
	}

	sources := make(map[string]sourceEntry)
	for _, p := range append(defaultPaths, extraPaths...) {
		fileCfg, loadErr := clientcmd.LoadFromFile(p)
		if loadErr != nil {
			continue
		}
		isDefault := defaultSet[p]
		for name := range fileCfg.Contexts {
			if _, seen := sources[name]; !seen {
				sources[name] = sourceEntry{path: p, isDefault: isDefault}
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.rawConfig = cfg
	m.contexts = make([]KubeContext, 0, len(cfg.Contexts))

	for name, ctx := range cfg.Contexts {
		ns := ctx.Namespace
		if ns == "" {
			ns = "default"
		}
		kc := KubeContext{
			Name:      name,
			Cluster:   ctx.Cluster,
			User:      ctx.AuthInfo,
			Namespace: ns,
			Status:    StatusDisconnected,
		}
		if src, ok := sources[name]; ok {
			kc.SourcePath = src.path
			kc.IsDefault = src.isDefault
		}
		if conn, ok := m.connections[name]; ok {
			kc.Status = conn.Status
		}
		m.contexts = append(m.contexts, kc)
	}

	slox.Info(m.ctx, "loaded kubeconfigs", "contexts", len(m.contexts))
	return nil
}

func (m *Manager) ListContexts() []KubeContext {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]KubeContext, len(m.contexts))
	copy(out, m.contexts)

	for i := range out {
		if conn, ok := m.connections[out[i].Name]; ok {
			out[i].Status = conn.Status
			out[i].ServerVersion = conn.ServerVersion
			out[i].Provider = conn.Provider
		}
	}

	return out
}

func (m *Manager) Connect(ctx context.Context, contextName string) error {
	m.mu.Lock()
	if _, ok := m.connections[contextName]; ok {
		m.mu.Unlock()
		return nil
	}

	if m.rawConfig == nil {
		m.mu.Unlock()
		return fmt.Errorf("kubeconfigs not loaded")
	}

	rawCtx, ok := m.rawConfig.Contexts[contextName]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("context %q not found", contextName)
	}
	m.mu.Unlock()

	m.emitStatus(contextName, StatusConnecting)

	clientCfg := clientcmd.NewDefaultClientConfig(*m.rawConfig, &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	})

	restCfg, err := clientCfg.ClientConfig()
	if err != nil {
		m.emitStatus(contextName, StatusError)
		return fmt.Errorf("building rest config for %q: %w", contextName, err)
	}

	restCfg.WarningHandler = FilteredWarningHandler

	if m.config != nil && m.config.InsecureSkipTLSVerify {
		restCfg.TLSClientConfig.Insecure = true
		restCfg.TLSClientConfig.CAFile = ""
		restCfg.TLSClientConfig.CAData = nil
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		m.emitStatus(contextName, StatusError)
		return fmt.Errorf("creating clientset for %q: %w", contextName, err)
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		m.emitStatus(contextName, StatusError)
		return fmt.Errorf("creating dynamic client for %q: %w", contextName, err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		m.emitStatus(contextName, StatusError)
		return fmt.Errorf("creating discovery client for %q: %w", contextName, err)
	}

	connCtx, cancel := context.WithCancel(ctx)

	ns := rawCtx.Namespace
	if ns == "" {
		ns = "default"
	}

	conn := &Connection{
		KubeContext: KubeContext{
			Name:      contextName,
			Cluster:   rawCtx.Cluster,
			User:      rawCtx.AuthInfo,
			Namespace: ns,
			Status:    StatusConnected,
		},
		Config:    restCfg,
		Clientset: clientset,
		Dynamic:   dynClient,
		Discovery: disc,
		cancel:    cancel,
		connCtx:   connCtx,
	}

	m.mu.Lock()
	m.connections[contextName] = conn
	m.mu.Unlock()

	m.emitStatus(contextName, StatusConnected)

	return nil
}

// Activate starts monitoring and runs one-shot bootstrap (permissions, server
// version, metrics capability detection, resource discovery). Idempotent — a
// second call on an already-activated connection is a no-op.
func (m *Manager) Activate(ctx context.Context, contextName string) error {
	m.mu.Lock()
	conn, ok := m.connections[contextName]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("not connected to %q", contextName)
	}
	if conn.activated {
		m.mu.Unlock()
		return nil
	}
	monitorCtx, cancel := context.WithCancel(conn.connCtx)
	conn.monitorCancel = cancel
	conn.activated = true
	m.mu.Unlock()

	go m.healthMonitor(monitorCtx, conn)
	go m.fetchAndStorePermissions(monitorCtx, contextName, conn)
	go m.startHealthPoller(monitorCtx, contextName, conn)
	go m.emitDiscovery(monitorCtx, contextName)
	go func() {
		sv, err := conn.Clientset.Discovery().ServerVersion()
		if err != nil {
			slox.Warn(m.ctx, "server version fetch failed", "context", contextName, "error", err)
			return
		}
		provider := detectProvider(conn.Cluster, sv.GitVersion)
		m.mu.Lock()
		if c, ok := m.connections[contextName]; ok {
			c.ServerVersion = sv.GitVersion
			c.Provider = provider
		}
		m.mu.Unlock()
		m.emitEvent(fmt.Sprintf("metadata:%s:cluster", contextName), map[string]string{
			"serverVersion": sv.GitVersion,
			"provider":      provider,
		})
	}()
	go func() {
		mc, err := metricsClientset.NewForConfig(conn.Config)
		if err != nil {
			slox.Warn(m.ctx, "metrics client creation failed", "context", contextName, "error", err)
			return
		}
		slox.Debug(m.ctx, "detecting metrics sources", "context", contextName)
		msProvider := metrics.NewMetricsServerProvider(mc.MetricsV1beta1(), conn.Discovery)
		cap := metrics.MetricsCapability{
			HasMetricsServer: msProvider.Available(),
		}
		slox.Debug(m.ctx, "metrics-server detection result", "context", contextName, "available", cap.HasMetricsServer)

		var manualURL string
		if m.config != nil {
			if mc, ok := m.config.Metrics[contextName]; ok && mc != nil {
				manualURL = mc.PrometheusURL
			}
		}
		if promURL, found := metrics.DetectPrometheus(m.ctx, conn.Clientset, conn.Discovery, conn.Dynamic, conn.Config, manualURL); found {
			cap.HasPrometheus = true
			cap.PrometheusURL = promURL
		}
		slox.Debug(m.ctx, "metrics detection complete", "context", contextName, "hasMetricsServer", cap.HasMetricsServer, "hasPrometheus", cap.HasPrometheus, "prometheusURL", cap.PrometheusURL)

		m.mu.Lock()
		if c, ok := m.connections[contextName]; ok {
			c.MetricsCapability = cap
		}
		m.mu.Unlock()
		m.emitEvent(fmt.Sprintf("metrics:%s:capabilities", contextName), cap)
	}()

	slox.Info(m.ctx, "cluster monitoring activated", "context", contextName)
	return nil
}

// Deactivate stops monitoring but keeps the connection alive. Idempotent.
// The cached Status is preserved (not reset) — last observed reality is more
// useful than a stale "connected" lie.
func (m *Manager) Deactivate(contextName string) {
	m.mu.Lock()
	conn, ok := m.connections[contextName]
	if !ok || !conn.activated {
		m.mu.Unlock()
		return
	}
	cancel := conn.monitorCancel
	conn.monitorCancel = nil
	conn.activated = false
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	slox.Info(m.ctx, "cluster monitoring deactivated", "context", contextName)
}

// IsActivated reports whether monitoring is running for the given context.
func (m *Manager) IsActivated(contextName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if conn, ok := m.connections[contextName]; ok {
		return conn.activated
	}
	return false
}

// discoverWithRetry runs discover with bounded exponential backoff, returning
// on the first success. It gives up after attempts tries or when ctx is
// cancelled, surfacing the last error.
func discoverWithRetry(ctx context.Context, attempts int, backoff time.Duration, discover func() ([]APIResource, error)) ([]APIResource, error) {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		res, err := discover()
		if err == nil {
			return res, nil
		}
		lastErr = err
		if attempt == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, lastErr
}

func (m *Manager) emitDiscovery(ctx context.Context, contextName string) {
	resources, err := discoverWithRetry(ctx, 5, 2*time.Second, func() ([]APIResource, error) {
		return m.DiscoverResources(contextName)
	})
	if err != nil {
		slox.Warn(m.ctx, "resource discovery failed", "context", contextName, "error", err)
		return
	}
	m.emitEvent(fmt.Sprintf("discovery:%s:resources", contextName), resources)
}

func (m *Manager) Disconnect(contextName string) error {
	m.Deactivate(contextName)

	m.mu.Lock()
	conn, ok := m.connections[contextName]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.connections, contextName)
	m.mu.Unlock()

	slox.Info(m.ctx, "cluster disconnecting", "context", contextName)
	if conn.cancel != nil {
		conn.cancel()
	}

	m.mu.RLock()
	hooks := append([]func(string){}, m.onDisconnect...)
	m.mu.RUnlock()
	m.runHooks(hooks, contextName)

	m.emitStatus(contextName, StatusDisconnected)
	return nil
}

func (m *Manager) RawConfig() *clientcmdapi.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rawConfig
}

func (m *Manager) GetConnection(contextName string) (*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, ok := m.connections[contextName]
	if !ok {
		return nil, fmt.Errorf("not connected to %q", contextName)
	}
	return conn, nil
}

func (m *Manager) GetMetricsCapability(contextName string) metrics.MetricsCapability {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if conn, ok := m.connections[contextName]; ok {
		return conn.MetricsCapability
	}
	return metrics.MetricsCapability{}
}

func (m *Manager) SetMetricsCapability(contextName string, cap metrics.MetricsCapability) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.connections[contextName]; ok {
		conn.MetricsCapability = cap
	}
}

func (m *Manager) ListNamespaces(ctx context.Context, contextName string) ([]string, error) {
	conn, err := m.GetConnection(contextName)
	if err != nil {
		return nil, err
	}

	nsList, err := conn.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces for %q: %w", contextName, err)
	}

	names := make([]string, len(nsList.Items))
	for i, ns := range nsList.Items {
		names[i] = ns.Name
	}
	return names, nil
}

func (m *Manager) CreateNamespace(ctx context.Context, contextName, name string) error {
	conn, err := m.GetConnection(contextName)
	if err != nil {
		return err
	}
	_, err = conn.Clientset.CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}},
		metav1.CreateOptions{})
	if err == nil {
		slox.Info(m.ctx, "namespace created", "context", contextName, "namespace", name)
	}
	return err
}

func (m *Manager) DeleteNamespace(ctx context.Context, contextName, name string) error {
	conn, err := m.GetConnection(contextName)
	if err != nil {
		return err
	}
	err = conn.Clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err == nil {
		slox.Info(m.ctx, "namespace deleted", "context", contextName, "namespace", name)
	}
	return err
}

// APIResource describes a discoverable resource on the cluster, including
// optional metadata useful for rendering (printer columns, subresources,
// scale subresource paths for CRDs).
type APIResource struct {
	GVR            string                    `json:"gvr"`
	Kind           string                    `json:"kind"`
	Namespaced     bool                      `json:"namespaced"`
	Subresources   ResourceSubresources      `json:"subresources"`
	PrinterColumns []AdditionalPrinterColumn `json:"printerColumns,omitempty"`
	ScaleSpec      *ScaleSubresourceSpec     `json:"scaleSpec,omitempty"`
}

// ResourceSubresources captures which well-known subresources are supported.
type ResourceSubresources struct {
	Scale  bool `json:"scale"`
	Status bool `json:"status"`
}

// AdditionalPrinterColumn mirrors the CRD printer column definition used by
// kubectl's column rendering.
type AdditionalPrinterColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // "string" | "integer" | "number" | "boolean" | "date"
	Format      string `json:"format,omitempty"`
	Description string `json:"description,omitempty"`
	Priority    int32  `json:"priority"`    // 0 = visible by default
	JSONPath    string `json:"jsonPath"`
}

// ScaleSubresourceSpec captures the paths the CRD declared for its scale
// subresource. Defaults are "spec.replicas" / "status.replicas".
type ScaleSubresourceSpec struct {
	SpecReplicasPath   string `json:"specReplicasPath"`
	StatusReplicasPath string `json:"statusReplicasPath"`
}

func (m *Manager) DiscoverResources(contextName string) ([]APIResource, error) {
	conn, err := m.GetConnection(contextName)
	if err != nil {
		return nil, err
	}

	lists, err := conn.Discovery.ServerPreferredResources()
	if err != nil && len(lists) == 0 {
		return nil, err
	}

	var primary []APIResource
	for _, list := range lists {
		if list == nil {
			continue
		}
		subs := DetectSubresources(list)
		gv, gvErr := parseGroupVersion(list.GroupVersion)
		if gvErr != nil {
			slox.Warn(m.ctx, "discovery: invalid group/version", "gv", list.GroupVersion, "err", gvErr)
			continue
		}
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue
			}
			primary = append(primary, APIResource{
				GVR:          formatGVR(gv.group, gv.version, r.Name),
				Kind:         r.Kind,
				Namespaced:   r.Namespaced,
				Subresources: subs[r.Name],
			})
		}
	}

	crdMeta := m.fetchCRDMetadata(conn)
	for i := range primary {
		if md, ok := crdMeta[primary[i].GVR]; ok {
			primary[i].PrinterColumns = md.PrinterColumns
			primary[i].ScaleSpec = md.ScaleSpec
		}
	}

	m.mu.Lock()
	if m.discoveredResources == nil {
		m.discoveredResources = map[string][]APIResource{}
	}
	m.discoveredResources[contextName] = primary
	m.mu.Unlock()

	return primary, nil
}

// HasScaleSubresource returns true when the given GVR declared a scale
// subresource during the most recent discovery pass for this context. Returns
// false when discovery hasn't run, the context is unknown, or the resource
// lacks scale.
func (m *Manager) HasScaleSubresource(contextName, gvr string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.discoveredResources[contextName] {
		if r.GVR == gvr {
			return r.Subresources.Scale
		}
	}
	return false
}

type groupVersion struct{ group, version string }

func parseGroupVersion(gv string) (groupVersion, error) {
	parts := strings.SplitN(gv, "/", 2)
	if len(parts) == 1 {
		if parts[0] == "" {
			return groupVersion{}, fmt.Errorf("empty group/version %q", gv)
		}
		return groupVersion{group: "core", version: parts[0]}, nil
	}
	if parts[0] == "" || parts[1] == "" {
		return groupVersion{}, fmt.Errorf("empty group or version in %q", gv)
	}
	return groupVersion{group: parts[0], version: parts[1]}, nil
}

func (m *Manager) fetchCRDMetadata(conn *Connection) map[string]CRDMetadata {
	if conn.Dynamic == nil {
		return map[string]CRDMetadata{}
	}
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	list, err := conn.Dynamic.Resource(crdGVR).List(m.ctx, metav1.ListOptions{})
	if err != nil {
		slox.Debug(m.ctx, "discovery: CRD list unavailable, skipping printer columns", "err", err)
		return map[string]CRDMetadata{}
	}

	crds := make([]apiextv1.CustomResourceDefinition, 0, len(list.Items))
	for i := range list.Items {
		var crd apiextv1.CustomResourceDefinition
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &crd); err != nil {
			slox.Debug(m.ctx, "discovery: CRD convert failed", "err", err)
			continue
		}
		crds = append(crds, crd)
	}
	return ExtractCRDMetadata(crds)
}

func (m *Manager) DisconnectAll() error {
	m.mu.RLock()
	names := make([]string, 0, len(m.connections))
	for name := range m.connections {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		if err := m.Disconnect(name); err != nil {
			return err
		}
	}
	return nil
}

// healthProbeTimeout bounds a single /healthz probe. A var so tests can
// shorten it. Without a deadline, a half-open TCP connection after laptop
// sleep wedges the health monitor loop permanently.
var healthProbeTimeout = 10 * time.Second

func probeHealthz(ctx context.Context, conn *Connection) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()
	return conn.Clientset.Discovery().RESTClient().Get().AbsPath("/healthz").Do(probeCtx).Raw()
}

func (m *Manager) healthMonitor(ctx context.Context, conn *Connection) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			body, err := probeHealthz(ctx, conn)
			if err != nil {
				slox.Warn(m.ctx, "health check failed", "context", conn.Name, "error", err)
				m.mu.Lock()
				if c, ok := m.connections[conn.Name]; ok {
					c.Status = StatusError
				}
				m.mu.Unlock()
				m.emitStatus(conn.Name, StatusError)
			} else if strings.TrimSpace(string(body)) == "ok" {
				m.mu.Lock()
				if c, ok := m.connections[conn.Name]; ok && c.Status != StatusConnected {
					c.Status = StatusConnected
					m.mu.Unlock()
					m.emitStatus(conn.Name, StatusConnected)
					// Connection recovered from an error — re-run discovery so
					// CRDs (and any other resources) added or missed during the
					// outage are picked up.
					go m.emitDiscovery(ctx, conn.Name)

					m.mu.RLock()
					recoveryHooks := append([]func(string){}, m.onRecovery...)
					m.mu.RUnlock()
					go m.runHooks(recoveryHooks, conn.Name)
				} else {
					m.mu.Unlock()
				}
			}
		}
	}
}

func (m *Manager) emitStatus(contextName string, status ConnectionStatus) {
	if m.emitEvent != nil {
		m.emitEvent(fmt.Sprintf("status:%s:connection", contextName), status.String())
	}
	slox.Info(m.ctx, "cluster status changed", "context", contextName, "status", status)
}

func (m *Manager) startHealthPoller(ctx context.Context, contextName string, conn *Connection) {
	check := func() {
		// CheckHealth makes 5 sequential API calls; bound the whole batch so a
		// dead socket surfaces as an error instead of wedging the poller.
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

func (m *Manager) fetchAndStorePermissions(ctx context.Context, contextName string, conn *Connection) {
	perms := FetchPermissions(ctx, conn.Clientset)
	m.mu.Lock()
	if c, ok := m.connections[contextName]; ok {
		c.Permissions = perms
	}
	m.mu.Unlock()
	m.emitEvent(fmt.Sprintf("cluster:%s:permissions", contextName), perms)
}
