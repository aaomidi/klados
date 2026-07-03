package enrichers_test

import (
	"testing"

	"github.com/MarvinJWendt/testza"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Vilsol/klados/internal/resource/enrichers"
)

func TestJobEnricher_StatusDisplay_Complete(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"completions": int64(1)},
		"status": map[string]any{
			"succeeded": int64(1),
			"conditions": []any{
				map[string]any{"type": "Complete", "status": "True"},
			},
		},
	}}

	e := &enrichers.JobEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))

	display, _, _ := unstructured.NestedString(obj.Object, "status", "statusDisplay")
	testza.AssertEqual(t, "Complete", display)
}

func TestJobEnricher_StatusDisplay_Failed(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"completions": int64(1)},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Failed", "status": "True"},
			},
		},
	}}

	e := &enrichers.JobEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))

	display, _, _ := unstructured.NestedString(obj.Object, "status", "statusDisplay")
	testza.AssertEqual(t, "Failed", display)
}

func TestJobEnricher_StatusDisplay_Running(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec":   map[string]any{"completions": int64(1)},
		"status": map[string]any{},
	}}

	e := &enrichers.JobEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))

	display, _, _ := unstructured.NestedString(obj.Object, "status", "statusDisplay")
	testza.AssertEqual(t, "Running", display)
}
