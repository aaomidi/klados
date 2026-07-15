# Pod Death Forensics — Design

**Date:** 2026-07-15
**Status:** Approved

## Problem

It is hard to identify why a pod died for non-standard reasons (OOMKilled,
evictions, probe failures, crash loops). Today:

- The pod list **Status** column shows only `status.phase`, so a pod stuck in
  `CrashLoopBackOff`, looping on OOM kills, or evicted shows as `Running` or a
  bare `Failed` — the same as `kubectl` would if it didn't compute a richer
  status.
- On the detail page, crash forensics exist but are **scattered**: you must open
  the correct `ContainerCard`, know that exit `137` means SIGKILL/OOM, and
  separately check the Events tab, correlating by hand.
- Exit codes are shown raw (no decoding).
- Pod-level eviction reason/message is not surfaced prominently.

## Goal

Answer "why did this pod die / why is it unhealthy" in two places:

1. **Pod list** — a `kubectl`-accurate computed Status.
2. **Pod detail** — a single failure-forensics summary combining container
   termination states (with decoded exit codes), pod-level eviction
   reason/message, and correlated warning events.

## Approach (chosen: A)

Status computation lives in the Go **enricher** (list columns render via CEL,
which cannot express `kubectl`'s loop-with-precedence). Detail forensics live in
the **frontend** (the detail page already receives the full object and has an
events watch — synthesis there needs zero new backend surface).

Rejected: computing status in the enricher **plus** correlating events there
(enrichers run per-object on every watch delta and have no events access —
per-pod event fetches would hammer the API server); and computing status in TS
(breaks the three-stage pipeline, hides status from CEL columns/plugins).

## Components

### 1. Go — kubectl-style status in `PodEnricher`

`internal/resource/enrichers/pod.go` computes `status.statusDisplay` following
`kubectl`'s pod printer precedence:

1. `status.reason` if set (**Evicted**, **NodeLost**, **Preempted**, …).
2. **Init container walk** (in order): a still-running/failed init container
   yields `Init:<reason>` — `Init:CrashLoopBackOff`, `Init:Error`,
   `Init:OOMKilled`, `Init:ExitCode:N`, or `Init:i/total` while progressing.
   Native sidecars (init containers with `restartPolicy: Always`) that are
   started and ready are **skipped** (they are not part of init progress).
3. **Main container walk**: first container that is
   - waiting with a reason → that reason (`CrashLoopBackOff`, `ImagePullBackOff`,
     `CreateContainerConfigError`, `ErrImagePull`, …), or
   - terminated with a reason → that reason (`OOMKilled`, `Error`,
     `Completed`), or terminated without a reason → `ExitCode:N`.
   `Completed` is only the pod-level status if **all** containers terminated
   with exit 0; a running container makes the pod `Running`.
4. `metadata.deletionTimestamp` set → **Terminating** (overrides all except
   `NodeLost`).
5. Fallback → `status.phase` (existing behavior), or `Unknown` if empty.

The existing `readyDisplay` / `restartCount` computation stays. No descriptor
change — the Status column already renders `status.statusDisplay` as a badge.

The function must stay nil-safe: malformed statuses fall back to phase and never
return an error.

### 2. Frontend — exit-code decoding in `containers.ts`

New pure helper `decodeExitCode(code: number, reason?: string): string`:

| Code    | Decoded text                              |
|---------|-------------------------------------------|
| 0       | `success`                                 |
| 1       | `application error`                       |
| 126     | `command not executable`                  |
| 127     | `command not found`                       |
| 137     | `SIGKILL — OOMKilled or forced kill` (or plain `OOMKilled` when `reason` says so) |
| 139     | `SIGSEGV — segmentation fault`            |
| 143     | `SIGTERM — graceful shutdown`             |
| >128    | `signal N (SIG…)` — generic, N = code−128 |
| other   | `exit N`                                  |

When `reason` is already descriptive (e.g. `OOMKilled`), prefer it and append
the signal gloss. `ContainerCard`'s existing "exit N" / "Last exit" lines gain
the decoded text (inline short form + tooltip).

### 3. Frontend — `PodDiagnosisBanner`

New panel component, rendered at the top of the pod's detail Overview and shown
**only when something is wrong** (hidden for healthy running pods). Synthesizes,
in order:

- **Pod-level failure**: `status.reason` + `status.message` (evictions render
  the full node-pressure message).
- **Per-container problems**: each container currently waiting with a bad reason
  or terminated non-zero, plus `lastState.terminated` forensics — reason,
  decoded exit code, finished time, restart count. Each entry offers a
  **Previous logs** action opening `LogsPanel` with `previous=true` preselected
  for that container.
- **Recent warning events**: reuses `GetEvents(ctx, ns, uid)` + the
  `watch:{ctx}:core.v1.events:{ns}` live-watch pattern from `EventsPanel`,
  filtered to `type=Warning` (probe `Unhealthy`, `BackOff`, `FailedScheduling`,
  `OOMKilling`). Capped at the **5 most recent**, with a "view all" jump to the
  Events tab.

Mounted in `ResourceDetail.svelte` for `core.v1.pods` above the Overview panel.

## Data flow

No new backend endpoints, no new Wails events. The enricher change flows through
the existing List/Watch → ResourceStore → CEL badge pipeline. The banner
consumes the already-loaded pod object plus the existing `GetEvents` binding and
events watch.

## Error handling

- Enricher: nil-safe defaults throughout; unknown shapes fall back to phase.
- Banner: `GetEvents` failure is treated as "no events" (silent — forensics from
  the pod object still render).

## Testing (TDD — red first)

- **Go** `pod_test.go` (testza, no CGO): table-driven —
  CrashLoopBackOff, OOMKilled (via `lastState` and via current terminated),
  Evicted (`status.reason`), Init:Error, native-sidecar skip, Completed,
  Terminating (deletionTimestamp), ImagePullBackOff, exit-code fallback, and the
  existing Running/ready cases still pass.
- **vitest**: `decodeExitCode` table; `PodDiagnosisBanner` — evicted pod,
  crashlooping container, warning events list, healthy-pod-hidden; `ContainerCard`
  decoded-exit-code line.

## Out of scope

- Node-level pressure diagnosis beyond the eviction message on the pod.
- Historical/previous pod objects (only the live object + `lastState`).
- Changes to other resource types' status computation.
