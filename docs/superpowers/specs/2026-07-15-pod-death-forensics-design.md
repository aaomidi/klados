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

> **Revised 2026-07-15** after three parallel design reviews (kubectl accuracy,
> architecture fit, UX/scope). Changes: corrected the enricher status rules to
> match real kubectl; scoped the detail banner to **aggregation only** (pod-level
> failure + unhealthy-container summary + warning events) so it no longer
> duplicates `ContainerCard`; dropped the infeasible "Previous logs" preselect;
> corrected the mount point.

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

`internal/resource/enrichers/pod.go` computes `status.statusDisplay` mirroring
`kubectl`'s `printPod` precedence. The rules below were corrected against
kubectl's real `pkg/printers/internalversion/printers.go` after review.

1. Seed from `status.reason` if set (**Evicted**, **Preempted**, …), else
   `status.phase`.
2. **Init container walk** (in order): a not-yet-successfully-terminated init
   container yields, in priority:
   - terminated with `reason` → `Init:<reason>` (`Init:OOMKilled`, `Init:Error`),
   - terminated without reason → `Init:Signal:N` if `signal != 0`, else
     `Init:ExitCode:N`,
   - waiting with a non-empty, non-`PodInitializing` reason →
     `Init:<reason>` (`Init:CrashLoopBackOff`), else
   - still progressing → `Init:i/total`.
   Native sidecars (init containers with `restartPolicy: Always`) that are
   started and ready are **skipped** — they are not part of init progress. A
   successfully completed (exit 0) init container is skipped and the walk
   continues.
3. **Main container walk** — walk `containerStatuses` in **reverse** so the
   lowest-indexed problem container's reason wins (matches kubectl's
   overwrite-in-reverse loop). For each:
   - waiting with a reason → that reason (`CrashLoopBackOff`, `ImagePullBackOff`,
     `CreateContainerConfigError`, `ErrImagePull`, …),
   - terminated with a `reason` → that reason (`OOMKilled`, `Error`,
     `Completed`),
   - terminated without a reason → `Signal:N` if `signal != 0`, else
     `ExitCode:N`.
   Track whether any container is running. If the computed reason is `Completed`
   but a container is still running, the pod is `Running`; if `Running` but the
   pod has no `Ready` condition it becomes `NotReady` (kubectl's narrow
   Completed→Running / →NotReady rule — a normal partially-ready Running pod
   stays `Running`).
4. **Deletion:** if `metadata.deletionTimestamp` is set **and** the phase is not
   already terminal (Succeeded/Failed), the status is **Terminating** — but if
   `status.reason == "NodeLost"`, render **Unknown** instead. Terminal pods
   being deleted keep their computed status.
5. Fallback → `status.phase`, or `Unknown` if empty.

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

### 3. Frontend — `PodDiagnosisBanner` (aggregation only, no duplication)

New component that **aggregates** the pod's health without re-rendering the
per-container forensics that `ContainerCard` already shows (restart count,
waiting/terminated message, `lastState.terminated` reason + decoded exit +
age). Shown **only when the pod is unhealthy**, per this explicit predicate:

```
show = status.reason is set
     || any container status is waiting with a reason other than
        ContainerCreating / PodInitializing
     || any container status is terminated with exitCode != 0
     || any init container (non-sidecar) failed
```

A `Completed` pod (all exit 0) and a healthy `Running` pod do **not** show the
banner. `restartCount > 0` alone does not trigger it (the container card already
badges restarts).

Contents, in order:

- **Pod-level failure**: `status.reason` + `status.message` — the one thing not
  shown anywhere today (evictions render the full node-pressure message). This
  is the banner's primary justification.
- **Unhealthy-container summary**: a single line — "N of M containers unhealthy"
  — with a link that switches `activePanel` to `overview` (where the
  `ContainerCard`s live). No per-container forensics are re-rendered here.
- **Recent warning events**: `GetEvents(ctx, ns, uid)` for the initial list plus
  the demand-started `watch:{ctx}:core.v1.events:{ns}` live watch, replicating
  `EventsPanel`'s pattern exactly — `StartWatch(ctx, "core.v1.events", ns)` on
  mount, filter deltas by `involvedObject.uid === uid`, `StopWatch` on unmount.
  Filtered to `type === "Warning"`, capped at the **5 most recent**, with a
  "view all" link that switches `activePanel` to `events`.

**Mounting:** placed in `ResourceDetail.svelte` immediately after
`<ValidationWarningBanner {obj} />` (line 242) behind `{#if gvr === 'core.v1.pods'}`
— the only always-visible slot above the tab bar; Overview is a *tab*, not
always-visible, so the banner cannot live "inside" it. Receives `obj`, `ctxName`,
`namespace`, the `$derived` `uid`, and a `setActivePanel` callback for the jump
links. Must guard on empty `uid` (empty until `obj` loads) before starting the
watch.

**Known duplication trade-off:** when the user is on the Events tab, both the
banner and `EventsPanel` hold a `core.v1.events` watch for the same ns. Backend
`StartWatch` is refcounted (30s grace), so this is two client-side upsert loops
over one shared server watch — acceptable, not a new server watch. Noted rather
than optimized (sharing would require lifting events state into a store — out of
scope).

**Dropped from the original design** (were duplication / infeasible): the
per-container forensics block, and the "Previous logs" preselect action —
`LogsPanel` has no `previous` prop (`previous` is internal checkbox state), so
preselecting it would require threading a new prop through `ResourceDetail` and
the `openContainer` path for marginal value. Users open previous logs via the
existing checkbox on the container's Logs panel.

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
  Evicted (`status.reason`), Init:Error, `Init:Signal:N`, native-sidecar skip,
  Completed, running-container-beats-Completed → Running, `NotReady` (Running
  phase, no Ready condition), Terminating (deletionTimestamp + non-terminal
  phase), terminal-pod-being-deleted keeps status, NodeLost + deletionTimestamp
  → Unknown, ImagePullBackOff, `Signal:N` / `ExitCode:N` fallback, reverse-walk
  precedence (two bad containers → lowest index wins), and the existing
  Running/ready cases still pass.
- **vitest**: `decodeExitCode` table; `PodDiagnosisBanner` — evicted pod shows
  reason/message, N-containers-unhealthy summary + jump link, warning events
  list capped at 5, healthy-pod-hidden (predicate), Completed-pod-hidden;
  `ContainerCard` decoded-exit-code line.

## Out of scope

- Node-level pressure diagnosis beyond the eviction message on the pod.
- Historical/previous pod objects (only the live object + `lastState`).
- Changes to other resource types' status computation.
