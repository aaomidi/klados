package enrichers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// computePodStatus mirrors kubectl's printPod status column precedence.
func computePodStatus(obj *unstructured.Unstructured) string {
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	reason, _, _ := unstructured.NestedString(obj.Object, "status", "reason")
	status := phase
	if reason != "" {
		status = reason
	}

	specInit, _, _ := unstructured.NestedSlice(obj.Object, "spec", "initContainers")
	initStatuses, _, _ := unstructured.NestedSlice(obj.Object, "status", "initContainerStatuses")

	initializing := false
	for i, raw := range initStatuses {
		cs, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		term := nestedMap(cs, "state", "terminated")
		waiting := nestedMap(cs, "state", "waiting")
		switch {
		case term != nil && intField(term, "exitCode") == 0:
			continue // completed successfully
		case boolField(cs, "started") && boolField(cs, "ready") && isRestartableInit(specInit, i):
			continue // native sidecar, running
		case term != nil:
			r := stringField(term, "reason")
			switch {
			case r != "":
				status = "Init:" + r
			case intField(term, "signal") != 0:
				status = fmt.Sprintf("Init:Signal:%d", intField(term, "signal"))
			default:
				status = fmt.Sprintf("Init:ExitCode:%d", intField(term, "exitCode"))
			}
			initializing = true
		case waiting != nil && stringField(waiting, "reason") != "" && stringField(waiting, "reason") != "PodInitializing":
			status = "Init:" + stringField(waiting, "reason")
			initializing = true
		default:
			status = fmt.Sprintf("Init:%d/%d", i, len(specInit))
			initializing = true
		}
		break
	}

	if !initializing || conditionTrue(obj, "Initialized") {
		hasRunning := false
		main := mainStatuses(obj)
		for i := len(main) - 1; i >= 0; i-- {
			cs, ok := main[i].(map[string]any)
			if !ok {
				continue
			}
			term := nestedMap(cs, "state", "terminated")
			waiting := nestedMap(cs, "state", "waiting")
			running := nestedMap(cs, "state", "running")
			switch {
			case waiting != nil && stringField(waiting, "reason") != "":
				status = stringField(waiting, "reason")
			case term != nil && stringField(term, "reason") != "":
				status = stringField(term, "reason")
			case term != nil && intField(term, "signal") != 0:
				status = fmt.Sprintf("Signal:%d", intField(term, "signal"))
			case term != nil:
				status = fmt.Sprintf("ExitCode:%d", intField(term, "exitCode"))
			case running != nil && boolField(cs, "ready"):
				hasRunning = true
			}
		}
		if status == "Completed" && hasRunning {
			if conditionTrue(obj, "Ready") {
				status = "Running"
			} else {
				status = "NotReady"
			}
		}
	}

	_, deleting, _ := unstructured.NestedString(obj.Object, "metadata", "deletionTimestamp")
	if deleting {
		if reason == "NodeLost" {
			return "Unknown"
		}
		if phase != "Succeeded" && phase != "Failed" {
			return "Terminating"
		}
	}
	if status == "" {
		return "Unknown"
	}
	return status
}

func mainStatuses(obj *unstructured.Unstructured) []any {
	s, _, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")
	return s
}

func isRestartableInit(specInit []any, i int) bool {
	if i >= len(specInit) {
		return false
	}
	c, ok := specInit[i].(map[string]any)
	if !ok {
		return false
	}
	return stringField(c, "restartPolicy") == "Always"
}

func conditionTrue(obj *unstructured.Unstructured, condType string) bool {
	conds, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, raw := range conds {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stringField(c, "type") == condType {
			return stringField(c, "status") == "True"
		}
	}
	return false
}

func nestedMap(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func stringField(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func boolField(m map[string]any, k string) bool {
	v, _ := m[k].(bool)
	return v
}

func intField(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}
