package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Vilsol/slox"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/helm"
	"github.com/Vilsol/klados/internal/resource"
	"github.com/Vilsol/klados/internal/watcher"
)

// HelmGVR is the synthetic GVR string used everywhere klados refers to Helm
// releases. Must match the descriptor registered in resource.builtin.go.
const HelmGVR = "helm.v1.releases"

// HelmService exposes Helm verbs and lazy detail-tab fetches over Wails RPC.
// It owns the Helm backend, actions wrapper, and per-release SAR-gated calls.
type HelmService struct {
	ctx      context.Context
	cluster  *cluster.Manager
	backend  *helm.Backend
	actions  *helm.Actions
	clients  *helm.ClientCache
	descReg  *resource.Registry
	engine   *resource.ResourceEngine
	watchMgr *watcher.WatchManager
	watchSrc *helm.WatchSource

	appService  *AppService
	resourceSvc *ResourceService

	mu        sync.RWMutex
	available map[string]bool // contextName -> probe result (helm secrets detected)
}

// NewHelmService constructs the service. Wiring of cluster manager / resource
// registry / engine / watcher happens in ServiceStartup via the injected
// AppService + ResourceService (their own ServiceStartup is guaranteed to
// have populated those references by the time ours runs).
func NewHelmService(appSvc *AppService, resSvc *ResourceService) *HelmService {
	return &HelmService{
		appService:  appSvc,
		resourceSvc: resSvc,
		available:   make(map[string]bool),
	}
}

func (s *HelmService) Startup(ctx context.Context) error {
	s.ctx = ctx
	if s.appService == nil {
		return fmt.Errorf("helm service: app service not wired")
	}
	if s.resourceSvc == nil {
		return fmt.Errorf("helm service: resource service not wired")
	}
	s.cluster = s.appService.ClusterManager()
	if s.cluster == nil {
		return fmt.Errorf("helm service: cluster manager not available")
	}

	lister := &clusterSecretAdapter{cluster: s.cluster}
	s.clients = helm.NewClientCache()
	s.backend = helm.NewBackend(s.clients, lister)
	s.actions = helm.NewActions(s.clients, s.getterFor, slog.Default().Handler(), lister)
	s.watchSrc = helm.NewWatchSource(s.backend, lister)

	s.descReg = s.resourceSvc.Registry()
	s.engine = s.resourceSvc.Engine()
	s.watchMgr = s.resourceSvc.WatchMgr()

	if s.engine != nil {
		s.engine.RegisterVirtual(HelmGVR, s.backend)
	}
	if s.watchMgr != nil {
		s.watchMgr.RegisterVirtual(HelmGVR, s.watchSrc)
	}
	return nil
}

func (s *HelmService) Shutdown() error { return nil }

// Backend returns the Helm Backend. Exposed for wiring/tests.
//
//wails:ignore
func (s *HelmService) Backend() *helm.Backend { return s.backend }

// WatchSource returns the helm VirtualWatchSource. Exposed for wiring/tests.
//
//wails:ignore
func (s *HelmService) WatchSource() *helm.WatchSource { return s.watchSrc }

// OnClusterConnected is called by ClusterService after a successful connect.
// It probes for the existence of any helm release Secret in the cluster and
// records the result for downstream UI gating. Runs asynchronously.
//
//wails:ignore
func (s *HelmService) OnClusterConnected(ctx context.Context, contextName string) {
	if s.cluster == nil {
		return
	}
	conn, err := s.cluster.GetConnection(contextName)
	if err != nil {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	list, err := conn.Clientset.CoreV1().Secrets("").List(probeCtx, metav1.ListOptions{
		FieldSelector: "type=" + helm.ReleaseSecretType,
		Limit:         1,
	})
	if err != nil {
		slox.Debug(s.ctx, "helm: probe failed", "context", contextName, "error", err)
		return
	}
	hasReleases := len(list.Items) > 0
	s.mu.Lock()
	s.available[contextName] = hasReleases
	s.mu.Unlock()
	if s.descReg != nil {
		s.descReg.SetAvailable(HelmGVR, true)
	}
}

// HasReleases returns the cached probe result for a context. Returns false if
// the context has not been probed.
//
//wails:ignore
func (s *HelmService) HasReleases(contextName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available[contextName]
}

// ---- RPC option types -------------------------------------------------------

type HelmRollbackOpts struct {
	Wait         bool `json:"wait"`
	Timeout      int  `json:"timeout"`
	DisableHooks bool `json:"disableHooks"`
}

type HelmUninstallOpts struct {
	Wait         bool `json:"wait"`
	Timeout      int  `json:"timeout"`
	DisableHooks bool `json:"disableHooks"`
	KeepHistory  bool `json:"keepHistory"`
}

type HelmTestOpts struct {
	Timeout int      `json:"timeout"`
	Filters []string `json:"filters,omitempty"`
}

type HelmPermissions struct {
	CanRollback    bool `json:"canRollback"`
	CanUninstall   bool `json:"canUninstall"`
	CanTest        bool `json:"canTest"`
	CanForceDelete bool `json:"canForceDelete"`
}

// ---- Verb RPCs --------------------------------------------------------------

func (s *HelmService) Rollback(ctx context.Context, contextName, namespace, releaseName string, revision int, opts HelmRollbackOpts) error {
	if err := s.requireAccess(ctx, contextName, namespace, "patch", "", "secrets"); err != nil {
		return err
	}
	return s.actions.Rollback(ctx, contextName, namespace, releaseName, revision, helm.RollbackOpts{
		Wait:         opts.Wait,
		Timeout:      time.Duration(opts.Timeout) * time.Second,
		DisableHooks: opts.DisableHooks,
	})
}

func (s *HelmService) Uninstall(ctx context.Context, contextName, namespace, releaseName string, opts HelmUninstallOpts) error {
	if err := s.requireAccess(ctx, contextName, namespace, "delete", "", "secrets"); err != nil {
		return err
	}
	return s.actions.Uninstall(ctx, contextName, namespace, releaseName, helm.UninstallOpts{
		Wait:         opts.Wait,
		Timeout:      time.Duration(opts.Timeout) * time.Second,
		DisableHooks: opts.DisableHooks,
		KeepHistory:  opts.KeepHistory,
	})
}

func (s *HelmService) Test(ctx context.Context, contextName, namespace, releaseName string, opts HelmTestOpts) (helm.TestResult, error) {
	if err := s.requireAccess(ctx, contextName, namespace, "create", "", "pods"); err != nil {
		return helm.TestResult{}, err
	}
	return s.actions.Test(ctx, contextName, namespace, releaseName, helm.TestOpts{
		Timeout: time.Duration(opts.Timeout) * time.Second,
		Filters: opts.Filters,
	})
}

func (s *HelmService) ForceDeleteReleaseSecret(ctx context.Context, contextName, namespace, releaseName string, revision int) error {
	if err := s.requireAccess(ctx, contextName, namespace, "delete", "", "secrets"); err != nil {
		return err
	}
	return s.actions.ForceDeleteReleaseSecret(ctx, contextName, namespace, releaseName, revision)
}

// ---- Lazy detail-tab fetches ------------------------------------------------

func (s *HelmService) GetValues(ctx context.Context, contextName, namespace, releaseName string, computed bool, revision int) (string, error) {
	return s.backend.GetValues(ctx, contextName, namespace, releaseName, computed, revision)
}

func (s *HelmService) GetManifest(ctx context.Context, contextName, namespace, releaseName string, revision int) (string, error) {
	return s.backend.GetManifest(ctx, contextName, namespace, releaseName, revision)
}

func (s *HelmService) GetHistory(ctx context.Context, contextName, namespace, releaseName string) ([]helm.Revision, error) {
	return s.backend.GetHistory(ctx, contextName, namespace, releaseName)
}

func (s *HelmService) GetNotes(ctx context.Context, contextName, namespace, releaseName string, revision int) (string, error) {
	return s.backend.GetNotes(ctx, contextName, namespace, releaseName, revision)
}

func (s *HelmService) GetHooks(ctx context.Context, contextName, namespace, releaseName string, revision int) ([]helm.Hook, error) {
	return s.backend.GetHooks(ctx, contextName, namespace, releaseName, revision)
}

func (s *HelmService) GetOwnedResources(ctx context.Context, contextName, namespace, releaseName string, scanAll bool) ([]helm.OwnedRef, error) {
	getter := &engineResourceGetter{engine: s.engine, cluster: s.cluster, ctx: s.ctx}
	return s.backend.GetOwnedResources(ctx, contextName, namespace, releaseName, scanAll, getter)
}

func (s *HelmService) DiffRevisions(ctx context.Context, contextName, namespace, releaseName string, from, to int) (helm.RevisionDiff, error) {
	return s.backend.DiffRevisions(ctx, contextName, namespace, releaseName, from, to)
}

// GetReleasePermissions returns the per-verb RBAC allow flags for the
// (context, namespace, release) triple. Errors from individual SAR calls
// degrade gracefully: the specific verb is reported false but no error is
// surfaced to the caller.
func (s *HelmService) GetReleasePermissions(ctx context.Context, contextName, namespace, releaseName string) (HelmPermissions, error) {
	_ = releaseName // reserved for future per-release granularity; currently unused
	check := func(verb, group, res string) bool {
		ok, err := s.cluster.CheckAccess(ctx, contextName, namespace, verb, group, res)
		if err != nil {
			slox.Debug(s.ctx, "helm: SAR check failed", "verb", verb, "group", group, "resource", res, "error", err)
			return false
		}
		return ok
	}
	return HelmPermissions{
		CanRollback:    check("patch", "", "secrets"),
		CanUninstall:   check("delete", "", "secrets"),
		CanTest:        check("create", "", "pods"),
		CanForceDelete: check("delete", "", "secrets"),
	}, nil
}

// CleanupTestPods deletes pods labelled as Helm test hooks for the given
// release. Both the "helm.sh/hook=test-success" and "helm.sh/hook=test" labels
// are recognised; the annotation form is not labelled and is not selectable
// server-side, so we filter client-side by annotation as a fallback.
func (s *HelmService) CleanupTestPods(ctx context.Context, contextName, namespace, releaseName string) error {
	conn, err := s.cluster.GetConnection(contextName)
	if err != nil {
		return err
	}
	selector := fmt.Sprintf("app.kubernetes.io/managed-by=Helm,meta.helm.sh/release-name=%s", releaseName)
	pods, err := conn.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}
	var firstErr error
	for i := range pods.Items {
		p := &pods.Items[i]
		if !isHelmTestPod(p) {
			continue
		}
		if delErr := conn.Clientset.CoreV1().Pods(namespace).Delete(ctx, p.Name, metav1.DeleteOptions{}); delErr != nil && !k8serrors.IsNotFound(delErr) {
			if firstErr == nil {
				firstErr = delErr
			}
		}
	}
	return firstErr
}

// ---- Internal helpers -------------------------------------------------------

func (s *HelmService) requireAccess(ctx context.Context, contextName, namespace, verb, group, resource string) error {
	allowed, err := s.cluster.CheckAccess(ctx, contextName, namespace, verb, group, resource)
	if err != nil {
		return fmt.Errorf("rbac check failed: %w", err)
	}
	if !allowed {
		return fmt.Errorf("denied: %s %s/%s in %s/%s", verb, group, resource, contextName, namespace)
	}
	return nil
}

// getterFor returns a Helm RESTClientGetter wired to the kubeconfig path and
// context of the named cluster. Each call mints a fresh ConfigFlags; the
// underlying *action.Configuration is cached by ClientCache.
func (s *HelmService) getterFor(contextName, namespace string) (genericclioptions.RESTClientGetter, error) {
	if s.cluster == nil {
		return nil, fmt.Errorf("helm: cluster manager not configured")
	}
	conn, err := s.cluster.GetConnection(contextName)
	if err != nil {
		return nil, err
	}
	flags := genericclioptions.NewConfigFlags(true)
	if conn.SourcePath != "" {
		p := conn.SourcePath
		flags.KubeConfig = &p
	}
	c := contextName
	flags.Context = &c
	n := namespace
	flags.Namespace = &n
	return flags, nil
}

func isHelmTestPod(p *corev1.Pod) bool {
	if v, ok := p.Annotations["helm.sh/hook"]; ok {
		for _, hk := range splitCSV(v) {
			if hk == "test" || hk == "test-success" || hk == "test-failure" {
				return true
			}
		}
	}
	if v, ok := p.Labels["helm.sh/hook"]; ok {
		switch v {
		case "test", "test-success", "test-failure":
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			tok := s[start:i]
			// Trim spaces.
			j := 0
			for j < len(tok) && (tok[j] == ' ' || tok[j] == '\t') {
				j++
			}
			k := len(tok)
			for k > j && (tok[k-1] == ' ' || tok[k-1] == '\t') {
				k--
			}
			if k > j {
				out = append(out, tok[j:k])
			}
			start = i + 1
		}
	}
	return out
}

// ---- Adapters between helm package and cluster.Manager ----------------------

// clusterSecretAdapter satisfies helm's secretLister, secretDeleter, and
// secretWatcher interfaces on top of cluster.Manager.
type clusterSecretAdapter struct {
	cluster *cluster.Manager
}

func (a *clusterSecretAdapter) ListSecrets(ctx context.Context, contextName, namespace, fieldSelector, labelSelector string) ([]corev1.Secret, string, error) {
	conn, err := a.cluster.GetConnection(contextName)
	if err != nil {
		return nil, "", err
	}
	list, err := conn.Clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, "", err
	}
	return list.Items, list.ResourceVersion, nil
}

func (a *clusterSecretAdapter) DeleteSecret(ctx context.Context, contextName, namespace, name string) error {
	conn, err := a.cluster.GetConnection(contextName)
	if err != nil {
		return err
	}
	return conn.Clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (a *clusterSecretAdapter) SecretsClient(contextName string) (kubernetes.Interface, error) {
	conn, err := a.cluster.GetConnection(contextName)
	if err != nil {
		return nil, err
	}
	return conn.Clientset, nil
}

// engineResourceGetter implements helm.resourceGetter on top of the resource
// engine for live existence / label-fallback scans.
type engineResourceGetter struct {
	engine  *resource.ResourceEngine
	cluster *cluster.Manager
	ctx     context.Context
}

func (g *engineResourceGetter) Exists(ctx context.Context, contextName, gvr, namespace, name string) (bool, error) {
	if g.engine == nil {
		return false, nil
	}
	_, err := g.engine.Get(ctx, contextName, gvr, namespace, name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (g *engineResourceGetter) ListByLabel(ctx context.Context, contextName, gvr, namespace, labelSelector string) ([]map[string]any, error) {
	conn, err := g.cluster.GetConnection(contextName)
	if err != nil {
		return nil, err
	}
	parsed, err := resource.ParseGVR(gvr)
	if err != nil {
		return nil, err
	}
	list, err := conn.Dynamic.Resource(parsed).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, list.Items[i].Object)
	}
	return out, nil
}

func (g *engineResourceGetter) KnownGVRs(contextName string) []string {
	if g.cluster == nil {
		return nil
	}
	resources, err := g.cluster.DiscoverResources(contextName)
	if err != nil && len(resources) == 0 {
		return nil
	}
	if err != nil {
		slox.Warn(g.ctx, "using partial discovery results", "context", contextName, "error", err)
	}
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		if !r.Namespaced {
			continue
		}
		out = append(out, r.GVR)
	}
	return out
}

