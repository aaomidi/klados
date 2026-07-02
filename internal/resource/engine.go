package resource

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/slox"
)

type ConnectionProvider interface {
	GetConnection(contextName string) (*cluster.Connection, error)
}

// requestTimeout bounds a single List/Get so a dead socket surfaces as an
// error instead of a frozen UI. A var so tests can shorten it.
var requestTimeout = 30 * time.Second

// VirtualBackend implements List/Get for a synthetic GVR that isn't backed by
// the dynamic Kubernetes client (e.g. Helm releases). Engine dispatches to a
// registered backend before falling through to the dynamic path.
type VirtualBackend interface {
	List(ctx context.Context, contextName, namespace string) ([]map[string]any, string, error)
	Get(ctx context.Context, contextName, namespace, name string) (map[string]any, error)
}

type ResourceEngine struct {
	clusterMgr  ConnectionProvider
	enricherReg *EnricherRegistry

	virtMu   sync.RWMutex
	virtuals map[string]VirtualBackend
}

func NewResourceEngine(mgr ConnectionProvider, enricherReg *EnricherRegistry) *ResourceEngine {
	return &ResourceEngine{
		clusterMgr:  mgr,
		enricherReg: enricherReg,
		virtuals:    make(map[string]VirtualBackend),
	}
}

// RegisterVirtual associates a VirtualBackend with a GVR. Subsequent calls to
// List/Get for that GVR are dispatched to the backend before any dynamic
// client path. Safe for concurrent use.
func (e *ResourceEngine) RegisterVirtual(gvr string, backend VirtualBackend) {
	e.virtMu.Lock()
	defer e.virtMu.Unlock()
	e.virtuals[gvr] = backend
}

func (e *ResourceEngine) lookupVirtual(gvr string) (VirtualBackend, bool) {
	e.virtMu.RLock()
	defer e.virtMu.RUnlock()
	b, ok := e.virtuals[gvr]
	return b, ok
}

func ParseGVR(gvr string) (schema.GroupVersionResource, error) {
	lastDot := strings.LastIndex(gvr, ".")
	if lastDot == -1 {
		return schema.GroupVersionResource{}, fmt.Errorf("invalid GVR %q", gvr)
	}
	resource := gvr[lastDot+1:]
	rest := gvr[:lastDot]

	secondLastDot := strings.LastIndex(rest, ".")
	if secondLastDot == -1 {
		return schema.GroupVersionResource{}, fmt.Errorf("invalid GVR %q", gvr)
	}
	version := rest[secondLastDot+1:]
	group := rest[:secondLastDot]

	if group == "core" {
		group = ""
	}

	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}, nil
}

func (e *ResourceEngine) client(ctx context.Context, contextName, gvr, namespace string) (dynamic.ResourceInterface, error) {
	conn, err := e.clusterMgr.GetConnection(contextName)
	if err != nil {
		return nil, err
	}

	parsed, err := ParseGVR(gvr)
	if err != nil {
		return nil, err
	}

	ri := conn.Dynamic.Resource(parsed)
	if namespace != "" {
		return ri.Namespace(namespace), nil
	}
	return ri, nil
}

func (e *ResourceEngine) enrich(contextName, gvr string, item *unstructured.Unstructured) {
	for _, enricher := range e.enricherReg.GetAll(gvr) {
		_ = enricher.Enrich(contextName, item)
	}
}

func (e *ResourceEngine) List(ctx context.Context, contextName, gvr, namespace string) ([]map[string]any, string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	t0 := time.Now()

	if backend, ok := e.lookupVirtual(gvr); ok {
		items, rv, err := backend.List(ctx, contextName, namespace)
		if err != nil {
			return nil, "", err
		}
		// Virtual backends still pass through the enricher chain.
		for i := range items {
			u := &unstructured.Unstructured{Object: items[i]}
			e.enrich(contextName, gvr, u)
			items[i] = u.Object
		}
		slox.Debug(ctx, "ResourceEngine.List (virtual)",
			"gvr", gvr,
			"namespace", namespace,
			"items", len(items),
			"rv", rv,
			"total_ms", time.Since(t0).Milliseconds(),
		)
		return items, rv, nil
	}

	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return nil, "", err
	}
	tClient := time.Since(t0)

	list, err := client.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("listing %s: %w", gvr, err)
	}
	tList := time.Since(t0)

	rv := list.GetResourceVersion()
	result := make([]map[string]any, len(list.Items))
	for i := range list.Items {
		e.enrich(contextName, gvr, &list.Items[i])
		// Same trim as the watch path: list rows never render managedFields,
		// and the detail view re-fetches via Get for the full object.
		unstructured.RemoveNestedField(list.Items[i].Object, "metadata", "managedFields")
		result[i] = list.Items[i].Object
	}
	tEnrich := time.Since(t0)

	slox.Debug(ctx, "ResourceEngine.List",
		"gvr", gvr,
		"namespace", namespace,
		"items", len(result),
		"rv", rv,
		"client_ms", tClient.Milliseconds(),
		"k8s_list_ms", (tList - tClient).Milliseconds(),
		"enrich_ms", (tEnrich - tList).Milliseconds(),
		"total_ms", tEnrich.Milliseconds(),
	)

	return result, rv, nil
}

// ListRaw lists resources without running enrichers. Used by plugin host API
// to avoid re-entering the Wasm runtime mutex during command execution.
func (e *ResourceEngine) ListRaw(ctx context.Context, contextName, gvr, namespace string) ([]map[string]any, error) {
	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return nil, err
	}

	list, err := client.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", gvr, err)
	}

	result := make([]map[string]any, len(list.Items))
	for i := range list.Items {
		result[i] = list.Items[i].Object
	}
	return result, nil
}

func (e *ResourceEngine) Get(ctx context.Context, contextName, gvr, namespace, name string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	if backend, ok := e.lookupVirtual(gvr); ok {
		obj, err := backend.Get(ctx, contextName, namespace, name)
		if err != nil {
			return nil, err
		}
		u := &unstructured.Unstructured{Object: obj}
		e.enrich(contextName, gvr, u)
		return u.Object, nil
	}

	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return nil, err
	}

	obj, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting %s/%s: %w", gvr, name, err)
	}

	e.enrich(contextName, gvr, obj)
	return obj.Object, nil
}

func (e *ResourceEngine) Delete(ctx context.Context, contextName, gvr, namespace, name string) error {
	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return err
	}

	policy := metav1.DeletePropagationBackground
	if err := client.Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	}); err != nil {
		return err
	}
	slox.Info(ctx, "resource deleted", "gvr", gvr, "namespace", namespace, "name", name)
	return nil
}

func (e *ResourceEngine) ForceDelete(ctx context.Context, contextName, gvr, namespace, name string) error {
	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return err
	}

	grace := int64(0)
	policy := metav1.DeletePropagationBackground
	if err := client.Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &grace,
		PropagationPolicy:  &policy,
	}); err != nil {
		return err
	}
	slox.Info(ctx, "resource force-deleted", "gvr", gvr, "namespace", namespace, "name", name)
	return nil
}

type ApplyResult struct {
	GVR       string `json:"gvr"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Action    string `json:"action"` // "created" | "configured" | "unchanged"
	Error     string `json:"error,omitempty"`
}

func boolPtr(b bool) *bool { return &b }

func (e *ResourceEngine) Apply(ctx context.Context, contextName, gvr string, obj *unstructured.Unstructured) (*ApplyResult, error) {
	client, err := e.client(ctx, contextName, gvr, obj.GetNamespace())
	if err != nil {
		return nil, err
	}

	name := obj.GetName()
	result := &ApplyResult{GVR: gvr, Namespace: obj.GetNamespace(), Name: name}

	_, getErr := client.Get(ctx, name, metav1.GetOptions{})
	preExists := getErr == nil

	data, err := obj.MarshalJSON()
	if err != nil {
		result.Error = fmt.Sprintf("marshal: %v", err)
		return result, nil
	}

	_, patchErr := client.Patch(ctx, name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: "klados",
		Force:        boolPtr(true),
	})
	if patchErr != nil {
		result.Error = patchErr.Error()
		return result, nil
	}

	if preExists {
		result.Action = "configured"
	} else {
		result.Action = "created"
	}
	return result, nil
}

func (e *ResourceEngine) Create(ctx context.Context, contextName, gvr, namespace string, obj map[string]any) (map[string]any, error) {
	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{Object: obj}
	created, err := client.Create(ctx, u, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating %s: %w", gvr, err)
	}

	e.enrich(contextName, gvr, created)
	return created.Object, nil
}

func (e *ResourceEngine) Update(ctx context.Context, contextName, gvr, namespace string, obj map[string]any) (map[string]any, error) {
	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{Object: obj}
	updated, err := client.Update(ctx, u, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("updating %s/%s: %w", gvr, u.GetName(), err)
	}

	e.enrich(contextName, gvr, updated)
	return updated.Object, nil
}

func (e *ResourceEngine) Patch(ctx context.Context, contextName, gvr, namespace, name string, patchType types.PatchType, data []byte) (map[string]any, error) {
	client, err := e.client(ctx, contextName, gvr, namespace)
	if err != nil {
		return nil, err
	}

	updated, err := client.Patch(ctx, name, patchType, data, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("patching %s/%s: %w", gvr, name, err)
	}

	e.enrich(contextName, gvr, updated)
	return updated.Object, nil
}

// Scale updates a resource's replica count via the scale subresource. Works
// for any resource that declares a scale subresource, including CRDs.
func (e *ResourceEngine) Scale(ctx context.Context, contextName, gvr, namespace, name string, replicas int32) error {
	conn, err := e.clusterMgr.GetConnection(contextName)
	if err != nil {
		return err
	}
	parsed, err := ParseGVR(gvr)
	if err != nil {
		return err
	}

	current, err := conn.Dynamic.Resource(parsed).Namespace(namespace).Get(
		ctx, name, metav1.GetOptions{}, "scale",
	)
	if err != nil {
		return err
	}

	var scale autoscalingv1.Scale
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(current.Object, &scale); err != nil {
		return err
	}
	scale.Spec.Replicas = replicas
	// Preserve resourceVersion for optimistic concurrency — FromUnstructured
	// into a typed Scale can drop metadata fields the converter doesn't know.
	scale.ResourceVersion = current.GetResourceVersion()

	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&scale)
	if err != nil {
		return err
	}
	current.Object = u

	_, err = conn.Dynamic.Resource(parsed).Namespace(namespace).Update(
		ctx, current, metav1.UpdateOptions{}, "scale",
	)
	return err
}

// ScaleViaMergePatch is the legacy fallback for resources without a scale
// subresource. Preserves pre-Phase-7 behavior.
func (e *ResourceEngine) ScaleViaMergePatch(ctx context.Context, contextName, gvr, namespace, name string, replicas int32) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
	_, err := e.Patch(ctx, contextName, gvr, namespace, name, types.MergePatchType, patch)
	return err
}
