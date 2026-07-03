package cluster

import (
	"context"
	"testing"

	"github.com/MarvinJWendt/testza"
	openapi_v2 "github.com/google/gnostic-models/openapiv2"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	fakedyn "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/openapi"
	restclient "k8s.io/client-go/rest"
)

// stubDiscovery implements discovery.DiscoveryInterface with a fixed resource list.
type stubDiscovery struct {
	resources []*metav1.APIResourceList
}

func (s *stubDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return s.resources, nil
}

func (s *stubDiscovery) ServerPreferredNamespacedResources() ([]*metav1.APIResourceList, error) {
	return nil, nil
}

func (s *stubDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	for _, l := range s.resources {
		if l.GroupVersion == groupVersion {
			return l, nil
		}
	}
	return nil, nil
}

func (s *stubDiscovery) ServerGroups() (*metav1.APIGroupList, error) {
	return &metav1.APIGroupList{}, nil
}
func (s *stubDiscovery) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	return nil, s.resources, nil
}
func (s *stubDiscovery) ServerVersion() (*version.Info, error)        { return &version.Info{}, nil }
func (s *stubDiscovery) OpenAPISchema() (*openapi_v2.Document, error) { return nil, nil }
func (s *stubDiscovery) OpenAPIV3() openapi.Client                    { return nil }
func (s *stubDiscovery) RESTClient() restclient.Interface             { return nil }
func (s *stubDiscovery) WithLegacy() discovery.DiscoveryInterface     { return s }

func TestDiscoverResources_EmitsEnrichedPayload(t *testing.T) {
	disc := &stubDiscovery{
		resources: []*metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", Namespaced: true, Kind: "Pod"},
				},
			},
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{Name: "deployments", Namespaced: true, Kind: "Deployment"},
					{Name: "deployments/scale", Namespaced: true, Kind: "Scale"},
				},
			},
			{
				GroupVersion: "example.com/v1",
				APIResources: []metav1.APIResource{
					{Name: "widgets", Namespaced: true, Kind: "Widget"},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = apiextv1.AddToScheme(scheme)
	widgetCRD := &apiextv1.CustomResourceDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition"},
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextv1.CustomResourceDefinitionNames{Plural: "widgets", Kind: "Widget"},
			Scope: apiextv1.NamespaceScoped,
			Versions: []apiextv1.CustomResourceDefinitionVersion{{
				Name:   "v1",
				Served: true,
				AdditionalPrinterColumns: []apiextv1.CustomResourceColumnDefinition{
					{Name: "Replicas", Type: "integer", JSONPath: ".spec.replicas"},
				},
			}},
		},
	}
	gvrMap := map[schema.GroupVersionResource]string{
		{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}: "CustomResourceDefinitionList",
	}
	dyn := fakedyn.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrMap, widgetCRD)

	m := &Manager{
		connections: map[string]*Connection{
			"c": {
				Discovery: disc,
				Dynamic:   dyn,
			},
		},
		emitEvent: func(string, any) {},
		ctx:       context.Background(),
	}

	got, err := m.DiscoverResources("c")
	testza.AssertNoError(t, err)

	byGVR := map[string]APIResource{}
	for _, r := range got {
		byGVR[r.GVR] = r
	}

	testza.AssertEqual(t, "Pod", byGVR["core.v1.pods"].Kind)
	testza.AssertTrue(t, byGVR["apps.v1.deployments"].Subresources.Scale)

	widget, ok := byGVR["example.com.v1.widgets"]
	testza.AssertTrue(t, ok)
	testza.AssertEqual(t, 1, len(widget.PrinterColumns))
	testza.AssertEqual(t, "Replicas", widget.PrinterColumns[0].Name)
}

func TestHasScaleSubresource(t *testing.T) {
	m := &Manager{
		discoveredResources: map[string][]APIResource{
			"c": {
				{GVR: "apps.v1.deployments", Subresources: ResourceSubresources{Scale: true}},
				{GVR: "core.v1.pods", Subresources: ResourceSubresources{}},
			},
		},
	}
	testza.AssertTrue(t, m.HasScaleSubresource("c", "apps.v1.deployments"))
	testza.AssertFalse(t, m.HasScaleSubresource("c", "core.v1.pods"))
	testza.AssertFalse(t, m.HasScaleSubresource("c", "unknown"))
	testza.AssertFalse(t, m.HasScaleSubresource("other-ctx", "apps.v1.deployments"))
}
