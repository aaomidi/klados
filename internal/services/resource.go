package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Vilsol/slox"
	batchv1 "k8s.io/api/batch/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/resource"
	"github.com/Vilsol/klados/internal/watcher"
)

type ResourceService struct {
	appService  *AppService
	drainSvc    *DrainService
	engine      *resource.ResourceEngine
	watchMgr    *watcher.WatchManager
	registry    *resource.Registry
	enricherReg *resource.EnricherRegistry
	templateReg *resource.TemplateRegistry
	ctx         context.Context
}

func NewResourceService(appSvc *AppService, drainSvc *DrainService) *ResourceService {
	return &ResourceService{appService: appSvc, drainSvc: drainSvc}
}

func NewResourceServiceWithRegistry(reg *resource.TemplateRegistry) *ResourceService {
	return &ResourceService{templateReg: reg}
}

func (s *ResourceService) Startup(ctx context.Context) error {
	s.ctx = ctx

	reg, err := resource.NewRegistry()
	if err != nil {
		return err
	}

	enricherReg := resource.NewEnricherRegistry()
	if err := resource.RegisterBuiltin(reg, enricherReg, s.drainSvc); err != nil {
		return err
	}

	s.registry = reg
	s.enricherReg = enricherReg
	s.engine = resource.NewResourceEngine(s.appService.ClusterManager(), enricherReg)
	s.watchMgr = watcher.NewWatchManager(s.appService.ClusterManager(), enricherReg, s.appService.Emit, s.appService.Ctx())

	cm := s.appService.ClusterManager()
	cm.OnDisconnect(func(name string) { s.watchMgr.StopContext(name) })
	cm.OnRecovery(s.watchMgr.ResyncContext)

	templateReg := resource.NewTemplateRegistry()
	if err := resource.LoadBuiltinTemplates(templateReg); err != nil {
		return fmt.Errorf("load builtin templates: %w", err)
	}
	s.templateReg = templateReg
	return nil
}

func (s *ResourceService) Shutdown() error {
	if s.watchMgr != nil {
		s.watchMgr.StopAll()
	}
	return nil
}

type ListResult struct {
	Items           []map[string]any `json:"items"`
	ResourceVersion string           `json:"resourceVersion"`
}

func (s *ResourceService) ListResources(contextName, gvr, namespace string) ([]map[string]any, error) {
	namespace = s.sanitizeNamespace(gvr, namespace)
	items, _, err := s.engine.List(s.ctx, contextName, gvr, namespace)
	return items, err
}

// ListResourcesWithVersion returns list items along with the list's ResourceVersion
// so the caller can seed a subsequent watch without triggering a synthetic ADDED replay.
func (s *ResourceService) ListResourcesWithVersion(contextName, gvr, namespace string) (*ListResult, error) {
	namespace = s.sanitizeNamespace(gvr, namespace)
	items, rv, err := s.engine.List(s.ctx, contextName, gvr, namespace)
	if err != nil {
		return nil, err
	}
	return &ListResult{Items: items, ResourceVersion: rv}, nil
}

func (s *ResourceService) GetResource(contextName, gvr, namespace, name string) (map[string]any, error) {
	namespace = s.sanitizeNamespace(gvr, namespace)
	return s.engine.Get(s.ctx, contextName, gvr, namespace, name)
}

func (s *ResourceService) DeleteResource(contextName, gvr, namespace, name string) error {
	namespace = s.sanitizeNamespace(gvr, namespace)
	return s.engine.Delete(s.ctx, contextName, gvr, namespace, name)
}

func (s *ResourceService) StartWatch(contextName, gvr, namespace, resourceVersion string) error {
	namespace = s.sanitizeNamespace(gvr, namespace)
	return s.watchMgr.StartWatch(contextName, gvr, namespace, resourceVersion)
}

// sanitizeNamespace strips the namespace for cluster-scoped resources to prevent
// the dynamic client from hitting invalid namespaced API paths.
// sanitizeNamespace strips the namespace for cluster-scoped resources to prevent
// the dynamic client from hitting invalid namespaced API paths.
func (s *ResourceService) sanitizeNamespace(gvr, namespace string) string {
	if namespace == "" || s.registry == nil {
		return namespace
	}
	if desc, ok := s.registry.Get(gvr); ok && desc.ClusterScoped {
		return ""
	}
	return namespace
}

func (s *ResourceService) StopWatch(contextName, gvr, namespace string) {
	s.watchMgr.StopWatch(contextName, gvr, namespace)
}

func (s *ResourceService) ListAPIResources(contextName string) ([]cluster.APIResource, error) {
	discovered, err := s.appService.ClusterManager().DiscoverResources(contextName)
	if err != nil && len(discovered) == 0 {
		return nil, err
	}
	if err != nil {
		slox.Warn(s.ctx, "using partial discovery results", "context", contextName, "error", err)
	}
	return discovered, nil
}

func (s *ResourceService) GetDescriptors() []*resource.Descriptor {
	return s.registry.List()
}

func (s *ResourceService) Registry() *resource.Registry {
	return s.registry
}

func (s *ResourceService) EnricherRegistry() *resource.EnricherRegistry {
	return s.enricherReg
}

func (s *ResourceService) Engine() *resource.ResourceEngine {
	return s.engine
}

func (s *ResourceService) WatchMgr() *watcher.WatchManager {
	return s.watchMgr
}

//wails:ignore
func (s *ResourceService) TemplateRegistry() *resource.TemplateRegistry {
	return s.templateReg
}

func (s *ResourceService) CreateResource(contextName, gvr, namespace string, obj map[string]any) (map[string]any, error) {
	return s.engine.Create(s.ctx, contextName, gvr, namespace, obj)
}

func (s *ResourceService) UpdateResource(contextName, gvr, namespace string, obj map[string]any) (map[string]any, error) {
	return s.engine.Update(s.ctx, contextName, gvr, namespace, obj)
}

func (s *ResourceService) ForceDeleteResource(contextName, gvr, namespace, name string) error {
	return s.engine.ForceDelete(s.ctx, contextName, gvr, namespace, name)
}

func (s *ResourceService) GetEvents(contextName, namespace, uid string) ([]map[string]any, error) {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return nil, err
	}

	fieldSelector := fmt.Sprintf("involvedObject.uid=%s", uid)
	list, err := conn.Clientset.CoreV1().Events(namespace).List(s.ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	result := make([]map[string]any, len(list.Items))
	for i := range list.Items {
		data, err := json.Marshal(list.Items[i])
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &result[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *ResourceService) ScaleResource(contextName, gvr, namespace, name string, replicas int32) error {
	var mgr *cluster.Manager
	if s.appService != nil {
		mgr = s.appService.ClusterManager()
	}
	if mgr != nil && mgr.HasScaleSubresource(contextName, gvr) {
		err := s.engine.Scale(s.ctx, contextName, gvr, namespace, name, replicas)
		if err == nil {
			return nil
		}
		if !errors.IsNotFound(err) && !errors.IsMethodNotSupported(err) {
			return err
		}
		// 404 / 405 → subresource unexpectedly absent; fall through to MergePatch.
	}
	return s.engine.ScaleViaMergePatch(s.ctx, contextName, gvr, namespace, name, replicas)
}

func (s *ResourceService) ExpandPVC(contextName, namespace, name, newSize string) error {
	obj, err := s.engine.Get(s.ctx, contextName, "core.v1.persistentvolumeclaims", namespace, name)
	if err != nil {
		return err
	}

	phase, _, _ := unstructured.NestedString(obj, "status", "phase")
	if phase != "Bound" {
		return fmt.Errorf("PVC %s is not Bound (phase: %s)", name, phase)
	}

	currentSizeStr, _, _ := unstructured.NestedString(obj, "status", "capacity", "storage")
	if currentSizeStr == "" {
		currentSizeStr, _, _ = unstructured.NestedString(obj, "spec", "resources", "requests", "storage")
	}

	currentQty, err := apiresource.ParseQuantity(currentSizeStr)
	if err != nil {
		return fmt.Errorf("failed to parse current size %q: %w", currentSizeStr, err)
	}

	newQty, err := apiresource.ParseQuantity(newSize)
	if err != nil {
		return fmt.Errorf("invalid size %q: %w", newSize, err)
	}

	if newQty.Cmp(currentQty) <= 0 {
		return fmt.Errorf("new size %s must be larger than current size %s", newSize, currentSizeStr)
	}

	patch := fmt.Sprintf(`{"spec":{"resources":{"requests":{"storage":%q}}}}`, newSize)
	_, err = s.engine.Patch(s.ctx, contextName, "core.v1.persistentvolumeclaims", namespace, name, types.MergePatchType, []byte(patch))
	return err
}

func (s *ResourceService) RestartResource(contextName, gvr, namespace, name string) error {
	ts := time.Now().UTC().Format(time.RFC3339)
	patch := []byte(fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
		ts,
	))
	_, err := s.engine.Patch(s.ctx, contextName, gvr, namespace, name, types.MergePatchType, patch)
	return err
}

func (s *ResourceService) PauseRollout(contextName, namespace, name string) error {
	_, err := s.engine.Patch(s.ctx, contextName, "apps.v1.deployments", namespace, name, types.MergePatchType, []byte(`{"spec":{"paused":true}}`))
	return err
}

func (s *ResourceService) ResumeRollout(contextName, namespace, name string) error {
	_, err := s.engine.Patch(s.ctx, contextName, "apps.v1.deployments", namespace, name, types.MergePatchType, []byte(`{"spec":{"paused":false}}`))
	return err
}

type RolloutRevision struct {
	Revision int64          `json:"revision"`
	Name     string         `json:"name"`
	Labels   map[string]any `json:"labels,omitempty"`
}

func (s *ResourceService) GetRolloutHistory(contextName, gvr, namespace, name string) ([]RolloutRevision, error) {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return nil, err
	}

	switch gvr {
	case "apps.v1.deployments":
		obj, err := s.engine.Get(s.ctx, contextName, gvr, namespace, name)
		if err != nil {
			return nil, err
		}
		uid, _, _ := nestedString(obj, "metadata", "uid")

		rsList, err := conn.Clientset.AppsV1().ReplicaSets(namespace).List(s.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		var revisions []RolloutRevision
		for _, rs := range rsList.Items {
			for _, ref := range rs.OwnerReferences {
				if ref.UID == types.UID(uid) {
					revStr := rs.Annotations["deployment.kubernetes.io/revision"]
					rev, _ := strconv.ParseInt(revStr, 10, 64)
					revisions = append(revisions, RolloutRevision{
						Revision: rev,
						Name:     rs.Name,
					})
					break
				}
			}
		}
		return revisions, nil

	case "apps.v1.statefulsets", "apps.v1.daemonsets":
		obj, err := s.engine.Get(s.ctx, contextName, gvr, namespace, name)
		if err != nil {
			return nil, err
		}
		uid, _, _ := nestedString(obj, "metadata", "uid")

		crList, err := conn.Clientset.AppsV1().ControllerRevisions(namespace).List(s.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		var revisions []RolloutRevision
		for _, cr := range crList.Items {
			for _, ref := range cr.OwnerReferences {
				if ref.UID == types.UID(uid) {
					revisions = append(revisions, RolloutRevision{
						Revision: cr.Revision,
						Name:     cr.Name,
					})
					break
				}
			}
		}
		return revisions, nil
	}

	return nil, fmt.Errorf("rollback not supported for %s", gvr)
}

func (s *ResourceService) RollbackToRevision(contextName, gvr, namespace, name string, revision int64) error {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return err
	}

	switch gvr {
	case "apps.v1.deployments":
		if revision == 0 {
			history, err := s.GetRolloutHistory(contextName, gvr, namespace, name)
			if err != nil {
				return err
			}
			var current int64
			for _, r := range history {
				if r.Revision > current {
					current = r.Revision
				}
			}
			revision = current - 1
		}
		rsList, err := conn.Clientset.AppsV1().ReplicaSets(namespace).List(s.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		obj, err := s.engine.Get(s.ctx, contextName, gvr, namespace, name)
		if err != nil {
			return err
		}
		uid, _, _ := nestedString(obj, "metadata", "uid")
		for _, rs := range rsList.Items {
			revStr := rs.Annotations["deployment.kubernetes.io/revision"]
			rev, _ := strconv.ParseInt(revStr, 10, 64)
			if rev != revision {
				continue
			}
			owned := false
			for _, ref := range rs.OwnerReferences {
				if ref.UID == types.UID(uid) {
					owned = true
					break
				}
			}
			if !owned {
				continue
			}
			templateData, err := json.Marshal(rs.Spec.Template)
			if err != nil {
				return err
			}
			patch := fmt.Sprintf(`{"spec":{"template":%s}}`, string(templateData))
			_, err = s.engine.Patch(s.ctx, contextName, gvr, namespace, name, types.StrategicMergePatchType, []byte(patch))
			return err
		}
		return fmt.Errorf("revision %d not found", revision)

	case "apps.v1.statefulsets", "apps.v1.daemonsets":
		obj, err := s.engine.Get(s.ctx, contextName, gvr, namespace, name)
		if err != nil {
			return err
		}
		uid, _, _ := nestedString(obj, "metadata", "uid")
		crList, err := conn.Clientset.AppsV1().ControllerRevisions(namespace).List(s.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, cr := range crList.Items {
			if cr.Revision != revision {
				continue
			}
			owned := false
			for _, ref := range cr.OwnerReferences {
				if ref.UID == types.UID(uid) {
					owned = true
					break
				}
			}
			if !owned {
				continue
			}
			patchData, err := cr.Data.MarshalJSON()
			if err != nil {
				return err
			}
			_, err = s.engine.Patch(s.ctx, contextName, gvr, namespace, name, types.MergePatchType, patchData)
			return err
		}
		return fmt.Errorf("revision %d not found", revision)
	}

	return fmt.Errorf("rollback not supported for %s", gvr)
}

func (s *ResourceService) DeleteJobCascade(contextName, namespace, name string) error {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return err
	}
	policy := metav1.DeletePropagationForeground
	return conn.Clientset.BatchV1().Jobs(namespace).Delete(s.ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	})
}

func (s *ResourceService) DeleteJobOrphan(contextName, namespace, name string) error {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return err
	}
	policy := metav1.DeletePropagationOrphan
	return conn.Clientset.BatchV1().Jobs(namespace).Delete(s.ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	})
}

func (s *ResourceService) TriggerCronJob(contextName, namespace, name string) error {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return err
	}
	cj, err := conn.Clientset.BatchV1().CronJobs(namespace).Get(s.ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	suffix := fmt.Sprintf("-manual-%d", ts)
	maxPrefix := 63 - len(suffix)
	prefix := name
	if len(prefix) > maxPrefix {
		prefix = prefix[:maxPrefix]
	}
	jobName := prefix + suffix

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Annotations: map[string]string{
				"cronjob.kubernetes.io/instantiate": "manual",
			},
		},
		Spec: cj.Spec.JobTemplate.Spec,
	}
	job.Spec.Selector = nil
	delete(job.Spec.Template.Labels, "batch.kubernetes.io/controller-uid")
	delete(job.Spec.Template.Labels, "batch.kubernetes.io/job-name")

	_, err = conn.Clientset.BatchV1().Jobs(namespace).Create(s.ctx, job, metav1.CreateOptions{})
	return err
}

func (s *ResourceService) SuspendCronJob(contextName, namespace, name string) error {
	suspendTrue := true
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"suspend": suspendTrue}})
	_, err := s.engine.Patch(s.ctx, contextName, "batch.v1.cronjobs", namespace, name, types.MergePatchType, patch)
	return err
}

func (s *ResourceService) ResumeCronJob(contextName, namespace, name string) error {
	suspendFalse := false
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"suspend": suspendFalse}})
	_, err := s.engine.Patch(s.ctx, contextName, "batch.v1.cronjobs", namespace, name, types.MergePatchType, patch)
	return err
}

func (s *ResourceService) GetTemplates(contextName, gvr string) ([]resource.Template, error) {
	templates := s.templateReg.GetTemplates(gvr)
	if len(templates) > 0 {
		return templates, nil
	}
	if s.appService != nil {
		conn, err := s.appService.ClusterManager().GetConnection(contextName)
		if err == nil {
			ctx := s.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			t, schemaErr := s.templateReg.GenerateFromSchema(ctx, gvr, conn.Clientset.Discovery())
			if schemaErr == nil {
				return []resource.Template{t}, nil
			}
		}
	}
	return []resource.Template{bareSkeletonTemplate(gvr)}, nil
}

func (s *ResourceService) GetAllTemplateGVRs(contextName string) ([]string, error) {
	seen := make(map[string]struct{})
	for _, gvr := range s.templateReg.GetAllGVRs() {
		seen[gvr] = struct{}{}
	}
	if s.appService != nil {
		conn, err := s.appService.ClusterManager().GetConnection(contextName)
		if err == nil {
			lists, discErr := conn.Discovery.ServerPreferredResources()
			if discErr == nil {
				for _, list := range lists {
					for _, res := range list.APIResources {
						gvrStr, convErr := resource.GVRStringFromAPIResource(list.GroupVersion, res.Name)
						if convErr == nil {
							seen[gvrStr] = struct{}{}
						}
					}
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for gvr := range seen {
		out = append(out, gvr)
	}
	sort.Strings(out)
	return out, nil
}

func bareSkeletonTemplate(gvr string) resource.Template {
	return resource.Template{
		GVR:     gvr,
		Name:    "Default",
		Content: "apiVersion: \"\"\nkind: \"\"\nmetadata:\n  name: \"\"\nspec: {}\n",
		Source:  "schema",
	}
}

func (s *ResourceService) ApplyManifest(contextName, yamlContent string) ([]resource.ApplyResult, error) {
	discovered, err := s.appService.ClusterManager().DiscoverResources(contextName)
	if err != nil && len(discovered) == 0 {
		return nil, fmt.Errorf("discover resources: %w", err)
	}
	if err != nil {
		slox.Warn(s.ctx, "using partial discovery results", "context", contextName, "error", err)
	}

	decoder := yaml.NewDecoder(strings.NewReader(yamlContent))
	var results []resource.ApplyResult

	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			results = append(results, resource.ApplyResult{Error: fmt.Sprintf("decode YAML: %v", err)})
			continue
		}
		if len(raw) == 0 {
			continue
		}

		apiVersion, _, _ := unstructured.NestedString(raw, "apiVersion")
		kind, _, _ := unstructured.NestedString(raw, "kind")
		if apiVersion == "" || kind == "" {
			results = append(results, resource.ApplyResult{Error: "missing apiVersion or kind"})
			continue
		}

		gvr, resolveErr := resolveGVR(discovered, apiVersion, kind)
		if resolveErr != nil {
			results = append(results, resource.ApplyResult{Error: resolveErr.Error()})
			continue
		}

		obj := &unstructured.Unstructured{Object: raw}
		result, applyErr := s.engine.Apply(s.ctx, contextName, gvr, obj)
		if applyErr != nil {
			results = append(results, resource.ApplyResult{GVR: gvr, Error: applyErr.Error()})
			continue
		}
		results = append(results, *result)
	}

	return results, nil
}

func resolveGVR(resources []cluster.APIResource, apiVersion, kind string) (string, error) {
	var group, version string
	if idx := strings.LastIndex(apiVersion, "/"); idx != -1 {
		group = apiVersion[:idx]
		version = apiVersion[idx+1:]
	} else {
		version = apiVersion
	}
	groupKey := group
	if groupKey == "" {
		groupKey = "core"
	}
	prefix := fmt.Sprintf("%s.%s.", groupKey, version)
	for _, r := range resources {
		if r.Kind == kind && strings.HasPrefix(r.GVR, prefix) {
			return r.GVR, nil
		}
	}
	return "", fmt.Errorf("no resource found for kind %q (apiVersion: %q)", kind, apiVersion)
}

func nestedString(obj map[string]any, path ...string) (string, bool, error) {
	cur := any(obj)
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false, nil
		}
		cur = m[k]
	}
	s, ok := cur.(string)
	return s, ok, nil
}
