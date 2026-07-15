package enrichers_test

import (
	"testing"

	"github.com/MarvinJWendt/testza"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Vilsol/klados/internal/resource/enrichers"
)

func TestPodEnricher(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "test"},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "app"},
				map[string]any{"name": "sidecar"},
			},
		},
		"status": map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				map[string]any{"name": "app", "ready": true, "restartCount": int64(3)},
				map[string]any{"name": "sidecar", "ready": false, "restartCount": int64(1)},
			},
		},
	}}

	e := &enrichers.PodEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))

	readyDisplay, _, _ := unstructured.NestedString(obj.Object, "status", "readyDisplay")
	testza.AssertEqual(t, "1/2", readyDisplay)

	restartCount, _, _ := unstructured.NestedInt64(obj.Object, "status", "restartCount")
	testza.AssertEqual(t, int64(4), restartCount)

	statusDisplay, _, _ := unstructured.NestedString(obj.Object, "status", "statusDisplay")
	testza.AssertEqual(t, "Running", statusDisplay)
}

func TestPodEnricher_NoContainerStatuses(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "app"},
			},
		},
		"status": map[string]any{},
	}}

	e := &enrichers.PodEnricher{}
	testza.AssertNoError(t, e.Enrich("", obj))

	readyDisplay, _, _ := unstructured.NestedString(obj.Object, "status", "readyDisplay")
	testza.AssertEqual(t, "0/1", readyDisplay)
}

func TestComputePodStatus(t *testing.T) {
	cases := []struct {
		name   string
		status map[string]any
		meta   map[string]any
		want   string
	}{
		{
			name: "running and ready",
			status: map[string]any{
				"phase":      "Running",
				"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": true, "restartCount": int64(0),
						"state": map[string]any{"running": map[string]any{}}},
				},
			},
			want: "Running",
		},
		{
			name: "crashloopbackoff via waiting",
			status: map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": false,
						"state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}},
				},
			},
			want: "CrashLoopBackOff",
		},
		{
			name: "oomkilled via current terminated",
			status: map[string]any{
				"phase": "Failed",
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": false,
						"state": map[string]any{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137)}}},
				},
			},
			want: "OOMKilled",
		},
		{
			name: "oomkilled via lastState with waiting now",
			status: map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": false,
						"state":     map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}},
						"lastState": map[string]any{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137)}}},
				},
			},
			want: "CrashLoopBackOff",
		},
		{
			name:   "evicted via status.reason",
			status: map[string]any{"phase": "Failed", "reason": "Evicted", "message": "The node was low on resource: memory."},
			want:   "Evicted",
		},
		{
			name: "init error",
			status: map[string]any{
				"phase": "Pending",
				"initContainerStatuses": []any{
					map[string]any{"name": "migrate",
						"state": map[string]any{"terminated": map[string]any{"reason": "Error", "exitCode": int64(1)}}},
				},
			},
			meta: map[string]any{"initContainers": []any{map[string]any{"name": "migrate"}}},
			want: "Init:Error",
		},
		{
			name: "init signal fallback",
			status: map[string]any{
				"phase": "Pending",
				"initContainerStatuses": []any{
					map[string]any{"name": "migrate",
						"state": map[string]any{"terminated": map[string]any{"exitCode": int64(139), "signal": int64(11)}}},
				},
			},
			meta: map[string]any{"initContainers": []any{map[string]any{"name": "migrate"}}},
			want: "Init:Signal:11",
		},
		{
			name: "native sidecar skipped, pod running",
			status: map[string]any{
				"phase": "Running",
				"conditions": []any{
					map[string]any{"type": "Initialized", "status": "True"},
					map[string]any{"type": "Ready", "status": "True"},
				},
				"initContainerStatuses": []any{
					map[string]any{"name": "proxy", "started": true, "ready": true,
						"state": map[string]any{"running": map[string]any{}}},
				},
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": true,
						"state": map[string]any{"running": map[string]any{}}},
				},
			},
			meta: map[string]any{"initContainers": []any{map[string]any{"name": "proxy", "restartPolicy": "Always"}}},
			want: "Running",
		},
		{
			name: "completed all zero",
			status: map[string]any{
				"phase": "Succeeded",
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": false,
						"state": map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)}}},
				},
			},
			want: "Completed",
		},
		{
			name: "running container beats completed sibling",
			status: map[string]any{
				"phase":      "Running",
				"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
				"containerStatuses": []any{
					map[string]any{"name": "job", "ready": false,
						"state": map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)}}},
					map[string]any{"name": "app", "ready": true,
						"state": map[string]any{"running": map[string]any{}}},
				},
			},
			want: "Running",
		},
		{
			name: "notready when running but no ready condition",
			status: map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{"name": "job", "ready": false,
						"state": map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)}}},
					map[string]any{"name": "app", "ready": true,
						"state": map[string]any{"running": map[string]any{}}},
				},
			},
			want: "NotReady",
		},
		{
			name: "terminating on deletion of non-terminal pod",
			status: map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": true,
						"state": map[string]any{"running": map[string]any{}}},
				},
			},
			meta: map[string]any{"deletionTimestamp": "2026-07-15T00:00:00Z"},
			want: "Terminating",
		},
		{
			name: "terminal pod being deleted keeps status",
			status: map[string]any{
				"phase": "Succeeded",
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": false,
						"state": map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)}}},
				},
			},
			meta: map[string]any{"deletionTimestamp": "2026-07-15T00:00:00Z"},
			want: "Completed",
		},
		{
			name:   "nodelost plus deletion is unknown",
			status: map[string]any{"phase": "Running", "reason": "NodeLost"},
			meta:   map[string]any{"deletionTimestamp": "2026-07-15T00:00:00Z"},
			want:   "Unknown",
		},
		{
			name: "signal fallback for main container",
			status: map[string]any{
				"phase": "Failed",
				"containerStatuses": []any{
					map[string]any{"name": "app", "ready": false,
						"state": map[string]any{"terminated": map[string]any{"exitCode": int64(143), "signal": int64(15)}}},
				},
			},
			want: "Signal:15",
		},
		{
			name: "reverse walk lowest index wins",
			status: map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{"name": "a", "ready": false,
						"state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}},
					map[string]any{"name": "b", "ready": false,
						"state": map[string]any{"waiting": map[string]any{"reason": "ImagePullBackOff"}}},
				},
			},
			want: "CrashLoopBackOff",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := map[string]any{"name": "p"}
			spec := map[string]any{}
			for k, v := range tc.meta {
				if k == "initContainers" {
					spec["initContainers"] = v
				} else {
					meta[k] = v
				}
			}
			obj := &unstructured.Unstructured{Object: map[string]any{
				"metadata": meta, "spec": spec, "status": tc.status,
			}}
			e := &enrichers.PodEnricher{}
			testza.AssertNoError(t, e.Enrich("", obj))
			got, _, _ := unstructured.NestedString(obj.Object, "status", "statusDisplay")
			testza.AssertEqual(t, tc.want, got)
		})
	}
}
