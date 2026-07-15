package enrichers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type PodEnricher struct{}

func (e *PodEnricher) Enrich(_ string, obj *unstructured.Unstructured) error {
	containerStatuses, _, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")

	ready := 0
	restarts := int64(0)

	for _, cs := range containerStatuses {
		csMap, ok := cs.(map[string]any)
		if !ok {
			continue
		}
		if r, ok := csMap["ready"].(bool); ok && r {
			ready++
		}
		if rc, ok := csMap["restartCount"]; ok {
			switch v := rc.(type) {
			case int64:
				restarts += v
			case float64:
				restarts += int64(v)
			}
		}
	}

	total := len(containerStatuses)
	if total == 0 {
		specContainers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
		total = len(specContainers)
	}

	_ = unstructured.SetNestedField(obj.Object, fmt.Sprintf("%d/%d", ready, total), "status", "readyDisplay")
	_ = unstructured.SetNestedField(obj.Object, restarts, "status", "restartCount")
	_ = unstructured.SetNestedField(obj.Object, computePodStatus(obj), "status", "statusDisplay")
	return nil
}
