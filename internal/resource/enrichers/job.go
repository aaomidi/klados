package enrichers

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type JobEnricher struct{}

func (e *JobEnricher) Enrich(_ string, obj *unstructured.Unstructured) error {
	succeeded, _, _ := unstructured.NestedInt64(obj.Object, "status", "succeeded")
	completionsDesired, _, _ := unstructured.NestedInt64(obj.Object, "spec", "completions")

	_ = unstructured.SetNestedField(obj.Object, fmt.Sprintf("%d/%d", succeeded, completionsDesired), "status", "completionDisplay")

	startTimeStr, _, _ := unstructured.NestedString(obj.Object, "status", "startTime")
	completionTimeStr, _, _ := unstructured.NestedString(obj.Object, "status", "completionTime")

	durationDisplay := ""
	if startTimeStr != "" {
		start, err := time.Parse(time.RFC3339, startTimeStr)
		if err == nil {
			end := time.Now()
			if completionTimeStr != "" {
				if t, err := time.Parse(time.RFC3339, completionTimeStr); err == nil {
					end = t
				}
			}
			dur := end.Sub(start)
			durationDisplay = formatDuration(dur)
		}
	}

	_ = unstructured.SetNestedField(obj.Object, durationDisplay, "status", "durationDisplay")

	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	statusDisplay := "Running"
	for _, c := range conditions {
		cMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if s, _ := cMap["status"].(string); s != "True" {
			continue
		}
		if t, _ := cMap["type"].(string); t == "Complete" {
			statusDisplay = "Complete"
			break
		} else if t == "Failed" {
			statusDisplay = "Failed"
		}
	}
	_ = unstructured.SetNestedField(obj.Object, statusDisplay, "status", "statusDisplay")

	return nil
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
