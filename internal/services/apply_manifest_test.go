package services_test

import (
	"testing"

	"github.com/MarvinJWendt/testza"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynfake "k8s.io/client-go/dynamic/fake"
	kfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/resource"
	"github.com/Vilsol/klados/internal/services"
)

var fakeDiscoveryResources = []*metav1.APIResourceList{
	{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{Name: "configmaps", Kind: "ConfigMap", Namespaced: true},
		},
	},
}

func newApplyManifestService() *services.ResourceService {
	cs := kfake.NewSimpleClientset()
	fd := cs.Discovery().(*fakediscovery.FakeDiscovery)
	fd.Resources = fakeDiscoveryResources

	dynCS := dynfake.NewSimpleDynamicClient(scheme.Scheme)
	reg, _ := resource.NewRegistry()
	enricherReg := resource.NewEnricherRegistry()
	provider := &testConnProvider{conn: &cluster.Connection{
		Clientset: cs,
		Dynamic:   dynCS,
		Discovery: cs.Discovery(),
	}}
	eng := resource.NewResourceEngine(provider, enricherReg)
	return services.NewResourceServiceForApplyTest(cs, dynCS, cs.Discovery(), eng, reg, enricherReg)
}

const cmYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: default`

const cm2YAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm-2
  namespace: default`

func TestApplyManifest_SingleDocument(t *testing.T) {
	svc := newApplyManifestService()
	results, err := svc.ApplyManifest("ctx", cmYAML)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 1)
}

func TestApplyManifest_MultiDocument(t *testing.T) {
	svc := newApplyManifestService()
	yaml := cmYAML + "\n---\n" + cm2YAML
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 2)
}

func TestApplyManifest_LeadingSeparator(t *testing.T) {
	svc := newApplyManifestService()
	yaml := "---\n" + cmYAML
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 1)
}

func TestApplyManifest_TrailingSeparator(t *testing.T) {
	svc := newApplyManifestService()
	yaml := cmYAML + "\n---"
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 1)
}

func TestApplyManifest_EmptyDocsBetweenSeparators(t *testing.T) {
	svc := newApplyManifestService()
	yaml := cmYAML + "\n---\n---\n" + cm2YAML
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 2)
}

func TestApplyManifest_CommentOnlyDocSkipped(t *testing.T) {
	svc := newApplyManifestService()
	yaml := cmYAML + "\n---\n# just a comment\n---\n" + cm2YAML
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 2)
}

func TestApplyManifest_UnknownKind_ErrorInResult(t *testing.T) {
	svc := newApplyManifestService()
	yaml := `apiVersion: v1
kind: UnknownThing
metadata:
  name: x`
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 1)
	testza.AssertNotEqual(t, "", results[0].Error)
}

func TestApplyManifest_PartialFailureDoesNotAbort(t *testing.T) {
	svc := newApplyManifestService()
	// First doc: unknown kind (will fail), second doc: valid ConfigMap
	yaml := `apiVersion: v1
kind: UnknownThing
metadata:
  name: x
---
` + cmYAML
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 2)
	testza.AssertNotEqual(t, "", results[0].Error) // first failed
	// second was still attempted (may have error from fake client but was processed)
}

func TestApplyManifest_MissingApiVersion_ErrorInResult(t *testing.T) {
	svc := newApplyManifestService()
	yaml := `kind: ConfigMap
metadata:
  name: x`
	results, err := svc.ApplyManifest("ctx", yaml)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, results, 1)
	testza.AssertNotEqual(t, "", results[0].Error)
}
