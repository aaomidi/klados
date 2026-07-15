# Pod Death Forensics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make it easy to see *why* a pod died — surface kubectl-accurate status (OOMKilled, CrashLoopBackOff, Evicted…) in the pod list, decode exit codes on container cards, and aggregate pod-level failures + warning events into one banner on the detail page.

**Architecture:** kubectl-style status computation lives in the Go `PodEnricher` (list columns render via CEL, which can't express kubectl's loop-with-precedence). Exit-code decoding and the unhealthy predicate are pure frontend helpers in `containers.ts`. A thin `PodDiagnosisBanner` aggregates pod-level failure + warning events without re-rendering the per-container forensics `ContainerCard` already shows.

**Tech Stack:** Go 1.25 + `k8s.io/apimachinery/unstructured`, testza (Go tests, no CGO); Svelte 5 runes, vitest + `@testing-library/svelte`; Wails bindings.

**Spec:** `docs/superpowers/specs/2026-07-15-pod-death-forensics-design.md`

---

## File Structure

- **Create** `internal/resource/enrichers/pod_status.go` — `computePodStatus(obj)` implementing kubectl's `printPod` precedence + small unstructured accessors. Keeps `pod.go` focused.
- **Modify** `internal/resource/enrichers/pod.go` — call `computePodStatus` for `status.statusDisplay` (replaces the bare phase).
- **Modify** `internal/resource/enrichers/pod_test.go` — table-driven status cases.
- **Modify** `frontend/src/lib/kubernetes/containers.ts` — add `decodeExitCode()` and `podDiagnosis()` pure helpers.
- **Modify** `frontend/src/lib/__tests__/containers.test.ts` — tests for both helpers.
- **Modify** `frontend/src/lib/components/panels/ContainerCard.svelte` — append decoded exit code to the exit lines.
- **Create** `frontend/src/lib/components/panels/PodDiagnosisBanner.svelte` — aggregation banner.
- **Create** `frontend/src/lib/__tests__/PodDiagnosisBanner.svelte.test.ts` — banner tests.
- **Modify** `frontend/src/lib/components/ResourceDetail.svelte` — mount banner for `core.v1.pods` after `ValidationWarningBanner`.

---

## Task 1: kubectl-style pod status in the enricher

**Files:**
- Create: `internal/resource/enrichers/pod_status.go`
- Modify: `internal/resource/enrichers/pod.go:41-48`
- Test: `internal/resource/enrichers/pod_test.go`

- [ ] **Step 1: Write the failing table test**

Append to `internal/resource/enrichers/pod_test.go`:

```go
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
				"phase": "Running",
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
				"phase": "Running",
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
			name: "nodelost plus deletion is unknown",
			status: map[string]any{"phase": "Running", "reason": "NodeLost"},
			meta:  map[string]any{"deletionTimestamp": "2026-07-15T00:00:00Z"},
			want:  "Unknown",
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resource/enrichers/ -run TestComputePodStatus -v`
Expected: FAIL — most cases return the bare phase (e.g. `Running` instead of `CrashLoopBackOff`).

- [ ] **Step 3: Create `pod_status.go`**

Create `internal/resource/enrichers/pod_status.go`:

```go
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
		for i := len(mainStatuses(obj)) - 1; i >= 0; i-- {
			cs, ok := mainStatuses(obj)[i].(map[string]any)
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
```

- [ ] **Step 4: Wire it into `pod.go`**

In `internal/resource/enrichers/pod.go`, replace the phase block. Change lines 41-48:

```go
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase == "" {
		phase = "Unknown"
	}

	_ = unstructured.SetNestedField(obj.Object, fmt.Sprintf("%d/%d", ready, total), "status", "readyDisplay")
	_ = unstructured.SetNestedField(obj.Object, restarts, "status", "restartCount")
	_ = unstructured.SetNestedField(obj.Object, phase, "status", "statusDisplay")
	return nil
```

to:

```go
	_ = unstructured.SetNestedField(obj.Object, fmt.Sprintf("%d/%d", ready, total), "status", "readyDisplay")
	_ = unstructured.SetNestedField(obj.Object, restarts, "status", "restartCount")
	_ = unstructured.SetNestedField(obj.Object, computePodStatus(obj), "status", "statusDisplay")
	return nil
```

- [ ] **Step 5: Run status tests + existing enricher tests**

Run: `go test ./internal/resource/enrichers/ -run 'TestComputePodStatus|TestPodEnricher' -v`
Expected: PASS — all status cases green, and the existing `TestPodEnricher` (expects `Running`) and `TestPodEnricher_NoContainerStatuses` still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/resource/enrichers/pod_status.go internal/resource/enrichers/pod.go internal/resource/enrichers/pod_test.go
git commit -m "feat(resource): kubectl-style pod status in enricher"
```

---

## Task 2: `decodeExitCode` + `podDiagnosis` frontend helpers

**Files:**
- Modify: `frontend/src/lib/kubernetes/containers.ts` (append)
- Test: `frontend/src/lib/__tests__/containers.test.ts` (append)

- [ ] **Step 1: Write the failing tests**

Append to `frontend/src/lib/__tests__/containers.test.ts` (and add `decodeExitCode, podDiagnosis` to the import from `$lib/kubernetes/containers` at the top of the file):

```ts
describe("decodeExitCode", () => {
  it("decodes 137 as OOM when reason says so", () => {
    expect(decodeExitCode(137, "OOMKilled")).toBe("SIGKILL (out of memory)");
  });
  it("decodes 137 generically without reason", () => {
    expect(decodeExitCode(137)).toBe("SIGKILL — OOM or forced kill");
  });
  it("decodes common codes", () => {
    expect(decodeExitCode(143)).toBe("SIGTERM — graceful shutdown");
    expect(decodeExitCode(139)).toBe("SIGSEGV — segmentation fault");
    expect(decodeExitCode(127)).toBe("command not found");
    expect(decodeExitCode(126)).toBe("command not executable");
    expect(decodeExitCode(1)).toBe("application error");
    expect(decodeExitCode(0)).toBe("success");
  });
  it("decodes other signals generically", () => {
    expect(decodeExitCode(130)).toBe("signal 2");
  });
  it("falls back to exit N", () => {
    expect(decodeExitCode(2)).toBe("exit 2");
  });
});

describe("podDiagnosis", () => {
  it("hides for a healthy running pod", () => {
    const obj = {status: {phase: "Running", containerStatuses: [
      {name: "app", ready: true, state: {running: {}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(false);
  });
  it("hides for a completed pod", () => {
    const obj = {status: {phase: "Succeeded", containerStatuses: [
      {name: "app", ready: false, state: {terminated: {reason: "Completed", exitCode: 0}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(false);
  });
  it("shows pod-level eviction reason and message", () => {
    const obj = {status: {phase: "Failed", reason: "Evicted", message: "low on memory"}};
    const d = podDiagnosis(obj);
    expect(d.show).toBe(true);
    expect(d.reason).toBe("Evicted");
    expect(d.message).toBe("low on memory");
  });
  it("counts unhealthy containers", () => {
    const obj = {status: {phase: "Running", containerStatuses: [
      {name: "app", ready: false, state: {waiting: {reason: "CrashLoopBackOff"}}},
      {name: "side", ready: true, state: {running: {}}},
    ]}};
    const d = podDiagnosis(obj);
    expect(d.show).toBe(true);
    expect(d.unhealthy).toBe(1);
    expect(d.total).toBe(2);
  });
  it("ignores ContainerCreating as unhealthy", () => {
    const obj = {status: {phase: "Pending", containerStatuses: [
      {name: "app", ready: false, state: {waiting: {reason: "ContainerCreating"}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(false);
  });
  it("skips native sidecars but flags failed init", () => {
    const obj = {status: {phase: "Pending", initContainerStatuses: [
      {name: "proxy", started: true, ready: true, state: {running: {}}},
      {name: "migrate", state: {terminated: {reason: "Error", exitCode: 1}}},
    ]}};
    expect(podDiagnosis(obj).show).toBe(true);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend && npx vitest run src/lib/__tests__/containers.test.ts -t "decodeExitCode|podDiagnosis"`
Expected: FAIL — `decodeExitCode is not a function` / `podDiagnosis is not a function`.

- [ ] **Step 3: Implement the helpers**

Append to `frontend/src/lib/kubernetes/containers.ts`:

```ts
export function decodeExitCode(code: number, reason?: string): string {
  switch (code) {
    case 0:
      return "success";
    case 1:
      return "application error";
    case 126:
      return "command not executable";
    case 127:
      return "command not found";
    case 137:
      return reason === "OOMKilled" ? "SIGKILL (out of memory)" : "SIGKILL — OOM or forced kill";
    case 139:
      return "SIGSEGV — segmentation fault";
    case 143:
      return "SIGTERM — graceful shutdown";
  }
  if (code > 128 && code < 165) return `signal ${code - 128}`;
  return `exit ${code}`;
}

export interface PodDiagnosis {
  show: boolean;
  reason?: string;
  message?: string;
  unhealthy: number;
  total: number;
}

function isContainerUnhealthy(cs: KubernetesResource): boolean {
  const w = cs.state?.waiting;
  if (w?.reason && w.reason !== "ContainerCreating" && w.reason !== "PodInitializing") return true;
  const t = cs.state?.terminated;
  if (t && (t.exitCode ?? 0) !== 0) return true;
  return false;
}

export function podDiagnosis(obj: KubernetesResource | undefined): PodDiagnosis {
  const status = obj?.status ?? {};
  const statuses: KubernetesResource[] = status.containerStatuses ?? [];
  const initStatuses: KubernetesResource[] = status.initContainerStatuses ?? [];
  let unhealthy = 0;
  for (const cs of statuses) if (isContainerUnhealthy(cs)) unhealthy++;
  let initFailed = false;
  for (const cs of initStatuses) {
    if (cs.started && cs.ready) continue; // native sidecar
    if (isContainerUnhealthy(cs)) initFailed = true;
  }
  const reason: string | undefined = status.reason;
  const show = Boolean(reason) || unhealthy > 0 || initFailed;
  return {show, reason, message: status.message, unhealthy, total: statuses.length};
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend && npx vitest run src/lib/__tests__/containers.test.ts`
Expected: PASS — new and existing container tests green.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/kubernetes/containers.ts frontend/src/lib/__tests__/containers.test.ts
git commit -m "feat(frontend): decodeExitCode and podDiagnosis helpers"
```

---

## Task 3: Decoded exit code on `ContainerCard`

**Files:**
- Modify: `frontend/src/lib/components/panels/ContainerCard.svelte:6-15` (import), `:74-76` (badge), `:108-113` (last exit line)

- [ ] **Step 1: Add the import**

In `ContainerCard.svelte`, add `decodeExitCode` to the existing import from `$lib/kubernetes/containers` (currently lines 6-15):

```ts
  import {
    containerStateInfo,
    lastExit,
    decodeExitCode,
    probeSummaries,
    envSource,
    envFromSources,
    mountSource,
    resourceSummary,
    type RefLink,
  } from "$lib/kubernetes/containers";
```

- [ ] **Step 2: Decode the current-terminated exit in the status badge**

Replace lines 74-76:

```svelte
      <StatusBadge status={Boolean(status?.ready) || (info.kind === 'terminated' && info.exitCode === 0)} mode="pill">
        {info.label}{info.kind === 'terminated' && info.exitCode !== undefined ? ` (exit ${info.exitCode})` : ''}
      </StatusBadge>
```

with:

```svelte
      <StatusBadge
        status={Boolean(status?.ready) || (info.kind === 'terminated' && info.exitCode === 0)}
        mode="pill"
      >
        {info.label}{info.kind === 'terminated' && info.exitCode !== undefined
          ? ` (exit ${info.exitCode} · ${decodeExitCode(info.exitCode, info.label)})`
          : ''}
      </StatusBadge>
```

- [ ] **Step 3: Decode the "Last exit" line**

Replace lines 108-113:

```svelte
  <!-- Crash forensics -->
  {#if lx}
    <p class="text-xs mt-1.5 text-destructive" title={absoluteTime(lx.finishedAt)}>
      Last exit: {lx.reason} (exit {lx.exitCode}){lx.finishedAt ? ` · ${formatAge(lx.finishedAt)} ago` : ''}
    </p>
  {/if}
```

with:

```svelte
  <!-- Crash forensics -->
  {#if lx}
    <p class="text-xs mt-1.5 text-destructive" title={absoluteTime(lx.finishedAt)}>
      Last exit: {lx.reason} (exit {lx.exitCode} · {decodeExitCode(lx.exitCode, lx.reason)}){lx.finishedAt
        ? ` · ${formatAge(lx.finishedAt)} ago`
        : ''}
    </p>
  {/if}
```

- [ ] **Step 4: Type-check + existing card tests**

Run: `cd frontend && pnpm check && npx vitest run src/lib/__tests__/ContainerCard.svelte.test.ts`
Expected: PASS — no type errors, card tests green.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/panels/ContainerCard.svelte
git commit -m "feat(frontend): decode exit codes on ContainerCard"
```

---

## Task 4: `PodDiagnosisBanner` component

**Files:**
- Create: `frontend/src/lib/components/panels/PodDiagnosisBanner.svelte`
- Test: `frontend/src/lib/__tests__/PodDiagnosisBanner.svelte.test.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/__tests__/PodDiagnosisBanner.svelte.test.ts`:

```ts
import {describe, it, expect, vi, beforeEach} from "vitest";
import {render} from "@testing-library/svelte";
import PodDiagnosisBanner from "$lib/components/panels/PodDiagnosisBanner.svelte";

vi.mock("../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetEvents: vi.fn(() => Promise.resolve([])),
  StartWatch: vi.fn(() => Promise.resolve()),
  StopWatch: vi.fn(() => Promise.resolve()),
}));

function pod(status: Record<string, unknown>) {
  return {metadata: {uid: "u1", namespace: "default"}, status};
}

describe("PodDiagnosisBanner", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders nothing for a healthy pod", () => {
    const {container} = render(PodDiagnosisBanner, {
      props: {obj: pod({phase: "Running", containerStatuses: [{name: "app", ready: true, state: {running: {}}}]}),
        ctxName: "c", namespace: "default", uid: "u1", setActivePanel: () => {}},
    });
    expect(container.textContent?.trim()).toBe("");
  });

  it("shows eviction reason and message", () => {
    const {getByText} = render(PodDiagnosisBanner, {
      props: {obj: pod({phase: "Failed", reason: "Evicted", message: "The node was low on resource: memory."}),
        ctxName: "c", namespace: "default", uid: "u1", setActivePanel: () => {}},
    });
    expect(getByText("Evicted")).toBeTruthy();
    expect(getByText(/low on resource: memory/)).toBeTruthy();
  });

  it("shows unhealthy container summary with a jump link", async () => {
    const setActivePanel = vi.fn();
    const {getByText} = render(PodDiagnosisBanner, {
      props: {obj: pod({phase: "Running", containerStatuses: [
        {name: "app", ready: false, state: {waiting: {reason: "CrashLoopBackOff"}}},
        {name: "side", ready: true, state: {running: {}}},
      ]}), ctxName: "c", namespace: "default", uid: "u1", setActivePanel},
    });
    const link = getByText(/1 of 2 containers unhealthy/);
    expect(link).toBeTruthy();
    (link as HTMLElement).click();
    expect(setActivePanel).toHaveBeenCalledWith("overview");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/lib/__tests__/PodDiagnosisBanner.svelte.test.ts`
Expected: FAIL — cannot resolve `PodDiagnosisBanner.svelte` (file does not exist).

- [ ] **Step 3: Implement the component**

Create `frontend/src/lib/components/panels/PodDiagnosisBanner.svelte`:

```svelte
<script lang="ts">
  import {GetEvents, StartWatch, StopWatch} from "../../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {Events as WailsEvents} from "@wailsio/runtime";
  import {formatAge} from "$lib/utils/age";
  import {classifySeverity, eventTimestamp} from "$lib/event/event-columns";
  import type {EventItem} from "$lib/event/event-types";
  import {podDiagnosis} from "$lib/kubernetes/containers";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    ctxName,
    namespace,
    uid,
    setActivePanel,
  }: {
    obj: KubernetesResource;
    ctxName: string;
    namespace: string;
    uid: string;
    setActivePanel: (panel: string) => void;
  } = $props();

  const diag = $derived(podDiagnosis(obj));

  let events = $state<EventItem[]>([]);

  function tsMs(e: EventItem): number {
    const t = Date.parse(eventTimestamp(e));
    return Number.isNaN(t) ? 0 : t;
  }

  const warnings = $derived(
    [...events]
      .filter((e) => classifySeverity(e) === "Warning")
      .sort((a, b) => tsMs(b) - tsMs(a))
      .slice(0, 5),
  );

  function upsert(ev: EventItem) {
    const idx = events.findIndex((e) => e.metadata?.uid === ev.metadata?.uid);
    if (idx >= 0) events[idx] = ev;
    else events.push(ev);
  }

  $effect(() => {
    const currentCtx = ctxName;
    const currentNs = namespace;
    const currentUid = uid;
    events = [];
    if (!currentUid) return;

    StartWatch(currentCtx, "core.v1.events", currentNs, "").catch(() => {});
    const unsub = WailsEvents.On(`watch:${currentCtx}:core.v1.events:${currentNs}`, (wailsEvent: unknown) => {
      const data = (wailsEvent as {data?: {type?: string; object?: EventItem}}).data;
      const ev = data?.object;
      if (!ev || ev.involvedObject?.uid !== currentUid) return;
      if (data?.type === "DELETED") {
        events = events.filter((e) => e.metadata?.uid !== ev.metadata?.uid);
      } else {
        upsert(ev);
      }
    });

    GetEvents(currentCtx, currentNs, currentUid)
      .then((listed) => {
        for (const item of (listed ?? []) as EventItem[]) {
          if (!events.some((e) => e.metadata?.uid === item.metadata?.uid)) events.push(item);
        }
      })
      .catch(() => {});

    return () => {
      unsub?.();
      StopWatch(currentCtx, "core.v1.events", currentNs).catch(() => {});
    };
  });

  function absoluteTime(ts: string): string {
    return ts ? new Date(ts).toLocaleString() : "";
  }
</script>

{#if diag.show}
  <div class="shrink-0 border-b border-destructive/40 bg-destructive/5 px-4 py-2.5 text-sm">
    <div class="flex items-center gap-2">
      <span class="font-medium text-destructive">Pod unhealthy</span>
      {#if diag.reason}
        <span class="px-2 py-0.5 rounded-full bg-destructive/15 text-destructive text-xs font-medium">{diag.reason}</span>
      {/if}
    </div>

    {#if diag.message}
      <p class="mt-1 text-xs text-fg/80 break-words">{diag.message}</p>
    {/if}

    {#if diag.unhealthy > 0}
      <button
        type="button"
        onclick={() => setActivePanel('overview')}
        class="mt-1 text-xs text-accent hover:underline"
      >
        {diag.unhealthy} of {diag.total} containers unhealthy — view details
      </button>
    {/if}

    {#if warnings.length > 0}
      <div class="mt-2 rounded-md border border-border divide-y divide-border/60 overflow-hidden">
        {#each warnings as ev (ev.metadata?.uid)}
          {@const ts = eventTimestamp(ev)}
          <div class="grid grid-cols-[auto_1fr_auto] gap-x-3 px-2.5 py-1 items-baseline">
            <span class="text-xs font-mono text-amber-500">{ev.reason ?? ''}</span>
            <span class="text-xs text-muted break-words">{ev.message ?? ''}</span>
            <span class="text-xs text-muted whitespace-nowrap tabular-nums" title={absoluteTime(ts)}>
              {ts ? formatAge(ts) : '—'}
            </span>
          </div>
        {/each}
        <button
          type="button"
          onclick={() => setActivePanel('events')}
          class="w-full text-left px-2.5 py-1 text-xs text-accent hover:underline hover:bg-surface-hover"
        >
          View all events
        </button>
      </div>
    {/if}
  </div>
{/if}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/lib/__tests__/PodDiagnosisBanner.svelte.test.ts`
Expected: PASS — all three cases green.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/panels/PodDiagnosisBanner.svelte frontend/src/lib/__tests__/PodDiagnosisBanner.svelte.test.ts
git commit -m "feat(frontend): PodDiagnosisBanner aggregating pod failure and warning events"
```

---

## Task 5: Mount the banner on the pod detail page

**Files:**
- Modify: `frontend/src/lib/components/ResourceDetail.svelte` (import near line 46; markup after line 242)

- [ ] **Step 1: Import the banner**

In `ResourceDetail.svelte`, add near the other panel imports (e.g. after the `ContainersPanel` import, line 26):

```ts
  import PodDiagnosisBanner from "./panels/PodDiagnosisBanner.svelte";
```

- [ ] **Step 2: Mount it after the validation banner**

Immediately after line 242 (`<ValidationWarningBanner {obj} />`), add:

```svelte
  <!-- Pod failure diagnosis -->
  {#if gvr === 'core.v1.pods'}
    <PodDiagnosisBanner {obj} {ctxName} {namespace} {uid} setActivePanel={(p) => (activePanel = p)} />
  {/if}
```

- [ ] **Step 3: Type-check**

Run: `cd frontend && pnpm check`
Expected: PASS — no type errors. (`uid` is already `$derived` at line 232; `activePanel` is in scope.)

- [ ] **Step 4: Run the full frontend test suite**

Run: `cd frontend && pnpm test`
Expected: PASS — no regressions.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/ResourceDetail.svelte
git commit -m "feat(frontend): mount PodDiagnosisBanner on pod detail page"
```

---

## Task 6: Full verification gates

- [ ] **Step 1: Go tests (non-CGO fast set)**

Run: `go test ./internal/resource/... -v`
Expected: PASS.

- [ ] **Step 2: Frontend type-check + tests**

Run: `cd frontend && pnpm check && pnpm test`
Expected: PASS.

- [ ] **Step 3: Manual smoke (optional, needs a live cluster)**

Run `task dev`, open a namespace with a crashlooping / OOMKilled pod, confirm:
- Pod list Status column shows `CrashLoopBackOff` / `OOMKilled` / `Evicted` (not `Running`/`Failed`).
- Pod detail shows the red diagnosis banner with reason/message + warning events.
- Container card shows `exit 137 · SIGKILL (out of memory)`.

- [ ] **Step 4: Final commit if any fixups**

```bash
git add -A && git commit -m "chore: pod death forensics verification fixups"
```

---

## Notes for the implementer

- **Wails bindings unchanged** — no Go service signatures change, so `wails3 generate bindings` is NOT needed.
- **Import depth:** `PodDiagnosisBanner.svelte` lives in `panels/`, so the binding import is `../../../../bindings/...` (four `../`), matching `EventsPanel.svelte:2`. The test file lives in `__tests__/`, so its `vi.mock` path is `../../../bindings/...` (three `../`) — both resolve to the same `frontend/bindings/...` module, so the mock intercepts the component's import.
- **Panel key strings:** the banner's jump links use `"overview"` and `"events"` — these are the `activePanel` values used in `ResourceDetail.svelte`. Confirm `events` is a real panel key in the pod descriptor's `detailPanels` (it is, via the Events tab); `overview` is always present.
- **TDD discipline:** each task writes the test first and confirms red before implementing (project mandate).
