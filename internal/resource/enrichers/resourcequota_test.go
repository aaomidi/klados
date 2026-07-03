package enrichers_test

import (
	"testing"

	"github.com/MarvinJWendt/testza"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Vilsol/klados/internal/resource/enrichers"
)

func TestResourceQuotaEnricher_ResourceCount(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"hard": map[string]any{
				"cpu":             "4",
				"memory":          "8Gi",
				"pods":            "10",
				"requests.cpu":    "2",
				"requests.memory": "4Gi",
			},
		},
	}}
	e := &enrichers.ResourceQuotaEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))
	count, _, _ := unstructured.NestedString(obj.Object, "status", "resourceCount")
	testza.AssertEqual(t, "5", count)
}

func TestResourceQuotaEnricher_ResourceCount_Empty(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"hard": map[string]any{},
		},
	}}
	e := &enrichers.ResourceQuotaEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))
	count, _, _ := unstructured.NestedString(obj.Object, "status", "resourceCount")
	testza.AssertEqual(t, "0", count)
}
