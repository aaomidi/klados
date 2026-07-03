package enrichers_test

import (
	"testing"

	"github.com/MarvinJWendt/testza"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Vilsol/klados/internal/resource/enrichers"
)

func TestHPAEnricher_ReferenceDisplay(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{
				"kind": "Deployment",
				"name": "nginx",
			},
		},
	}}
	e := &enrichers.HPAEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))
	display, _, _ := unstructured.NestedString(obj.Object, "status", "referenceDisplay")
	testza.AssertEqual(t, "Deployment/nginx", display)
}

func TestHPAEnricher_TargetsDisplay_Resource(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{"kind": "Deployment", "name": "app"},
			"metrics": []any{
				map[string]any{
					"type": "Resource",
					"resource": map[string]any{
						"name": "cpu",
						"target": map[string]any{
							"averageUtilization": int64(80),
						},
					},
				},
			},
		},
		"status": map[string]any{
			"currentMetrics": []any{
				map[string]any{
					"type": "Resource",
					"resource": map[string]any{
						"name":                      "cpu",
						"currentAverageUtilization": int64(60),
					},
				},
			},
		},
	}}
	e := &enrichers.HPAEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))
	display, _, _ := unstructured.NestedString(obj.Object, "status", "targetsDisplay")
	testza.AssertEqual(t, "cpu: 60%/80%", display)
}

func TestHPAEnricher_TargetsDisplay_MissingCurrentMetrics(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{"kind": "Deployment", "name": "app"},
			"metrics": []any{
				map[string]any{
					"type": "Resource",
					"resource": map[string]any{
						"name": "cpu",
						"target": map[string]any{
							"averageUtilization": int64(80),
						},
					},
				},
			},
		},
	}}
	e := &enrichers.HPAEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))
	display, _, _ := unstructured.NestedString(obj.Object, "status", "targetsDisplay")
	testza.AssertEqual(t, "cpu: ?/80%", display)
}

func TestHPAEnricher_TargetsDisplay_MoreThan3(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{"kind": "Deployment", "name": "app"},
			"metrics": []any{
				map[string]any{"type": "Resource", "resource": map[string]any{"name": "cpu", "target": map[string]any{"averageUtilization": int64(80)}}},
				map[string]any{"type": "Resource", "resource": map[string]any{"name": "memory", "target": map[string]any{"averageUtilization": int64(70)}}},
				map[string]any{"type": "Resource", "resource": map[string]any{"name": "ephemeral-storage", "target": map[string]any{"averageUtilization": int64(60)}}},
				map[string]any{"type": "Resource", "resource": map[string]any{"name": "hugepages", "target": map[string]any{"averageUtilization": int64(50)}}},
			},
		},
	}}
	e := &enrichers.HPAEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))
	display, _, _ := unstructured.NestedString(obj.Object, "status", "targetsDisplay")
	testza.AssertContains(t, display, "...")
}

func TestHPAEnricher_TargetsDisplay_Pods(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{"kind": "Deployment", "name": "app"},
			"metrics": []any{
				map[string]any{
					"type": "Pods",
					"pods": map[string]any{
						"metric": map[string]any{"name": "packets-per-second"},
						"target": map[string]any{"averageValue": "1k"},
					},
				},
			},
		},
		"status": map[string]any{
			"currentMetrics": []any{
				map[string]any{
					"type": "Pods",
					"pods": map[string]any{
						"metric":  map[string]any{"name": "packets-per-second"},
						"current": map[string]any{"averageValue": "500"},
					},
				},
			},
		},
	}}
	e := &enrichers.HPAEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))
	display, _, _ := unstructured.NestedString(obj.Object, "status", "targetsDisplay")
	testza.AssertEqual(t, "packets-per-second: 500/1k", display)
}
