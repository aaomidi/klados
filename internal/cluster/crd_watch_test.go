package cluster

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestWatchCRDs_TriggersRediscoveryOnCRDChange(t *testing.T) {
	crdGVR := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		crdGVR: "CustomResourceDefinitionList",
	})
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

	crd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "things.xyz.io"},
	}}
	// Add blocks until the watch is established and consumed; run it in a
	// goroutine so a failed watch setup trips the timeout below instead of
	// hanging the test.
	go fw.Add(crd)

	select {
	case <-discoveryEvents:
	case <-time.After(2 * time.Second):
		t.Fatal("CRD add did not trigger re-discovery")
	}
}
