# Liveness Audit — 2026-07-02

Audit of every spot where a network outage, laptop sleep/resume, cert rotation, or
kubeconfig change can break the app's "liveness" — connections that don't get retried,
watches that die silently, stale clients, and UI that never refreshes.

Findings 1–3 were verified directly in source; the rest come from a four-agent sweep
over `internal/cluster`, `internal/watcher`, `internal/streaming|logs|exec|portforward|metrics`,
and `frontend/src`.

**Suggested fix order:** #3 first (config timeout, one line, unblocks all recovery),
then #1 + #2 together (shared watch-lifecycle root cause), then #7/#8 (frontend
re-list on reconnect).

> **Status 2026-07-02:** All critical + high findings (#1–#9) fixed — see
> `docs/superpowers/plans/2026-07-02-liveness-fixes.md` and the six commits
> `wyzptqzp`…`zzxsoqvv`. Medium findings (#10–#15) remain open. Follow-ups filed
> from review: discovery singleflight lacks a dirty-rerun flag (CRD event during
> a finishing loop is dropped); `loadContexts` has a brief unsubscribed window
> for status events; `ListRaw` (plugin host path) still has no request timeout;
> virtual watches still use `context.Background()`.

---

## Critical

### 1. A 410 Gone permanently kills a watch; the resync protocol can't revive it

When the resourceVersion expires (laptop sleep, long outage, API-server compaction),
`runWatch` emits the `:resync` event and returns — but never deletes `m.watches[key]`
(`internal/watcher/manager.go:200-203`, `:215-218`). The frontend resync handler
re-lists and calls `StartWatch` again (`frontend/src/lib/stores/resource.svelte.ts:114-117`),
but `StartWatch` sees the stale key and returns nil without spawning a goroutine
(`internal/watcher/manager.go:118-124`).

**Net effect:** one fresh snapshot, then the page silently stops updating forever.
Fires on exactly the sleep/outage scenarios this audit targets.

**Fixed:** `wyzptqzp` — `removeSelf` (pointer-identity guarded) runs before the
resync emit and on every goroutine exit; `Expired` now treated like `Gone`.

### 2. Watches aren't tied to the connection lifecycle — reconnect leaves them on a dead client

Watch goroutines run on `context.WithCancel(context.Background())`
(`internal/watcher/manager.go:168`) and capture `conn.Dynamic` at start (`:157`).
`watchMgr.StopAll()` is only called at app shutdown (`internal/services/resource.go:81`),
never from `cluster.Manager.Disconnect`. After a kubeconfig re-import or any
disconnect+connect (the flow added in commit 092289ab), old watch goroutines keep
retrying against the old transport/credentials every 5s, and re-calling `StartWatch`
is a no-op because the key still exists.

**Net effect:** cert rotation via re-import does not actually refresh live watches.

**Fixed:** `wyzptqzp` + `kkvkvvzk` — watches derive from `Connection.Context()`;
`OnDisconnect`/`OnRecovery` hooks drive `StopContext`/`ResyncContext`.

### 3. No timeout on `rest.Config` — a half-open TCP connection hangs everything, including the health monitor

`Connect` builds `restCfg` with no `Timeout`, `Dial`, or keepalive tuning
(`internal/cluster/manager.go:228-281`). The 15s health probe calls `.Do(ctx).Raw()`
with no per-probe deadline (`:706`), so a hung socket after sleep wedges the monitor
loop itself — the app never flips to StatusError, so the recovery path (which exists)
never triggers. Same exposure in `ResourceEngine` List/Get
(`internal/resource/engine.go:146,210`); watches set no `TimeoutSeconds`
(`internal/watcher/manager.go:194`), so a silent network drop without RST blocks
until the OS TCP timeout.

**Fixed:** `mkwuoszx` — `probeHealthz` (10s), `CheckHealth` bounded to 20s per
poll, engine List/Get bounded to 30s. Deliberately no global `rest.Config.Timeout`
(would kill watch/log streams); a wedged watch is recovered via `ResyncContext`
on health recovery instead.

---

## High — connection & discovery

### 4. Initial connect is never retried

`Connect` (`internal/cluster/manager.go:203-290`) and the startup reconnect
(`internal/services/app.go:83-95`) make one attempt. On failure no connection object
is stored, so the health monitor never runs — the context is permanently dead until
the user manually re-clicks.

**Fixed:** `vomwonsq` — startup reconnect retries 5× with exponential backoff.
(Note: `Connect` makes no network calls, so transient network failures are owned
by the health monitor + unbounded discovery retry, not this loop.)

### 5. Discovery gaps remain despite commit 78b367ae

- **Partial discovery treated as authoritative:** `DiscoverResources` returns
  `err == nil` when *any* API group lists succeed (`internal/cluster/manager.go:576-579`),
  so the retry wrapper considers a half-broken apiserver a success.
- **Bounded retry gives up permanently:** `discoverWithRetry` stops after 5 attempts
  / ~62s of backoff (`:404-423`).
- **No CRD watch:** re-discovery fires only on an error→connected transition (`:724`)
  and there is no watch on `apiextensions…customresourcedefinitions` — CRDs installed
  while the connection stays healthy never appear until a reconnect.

This is the "CRDs don't get loaded after connection issues" complaint from bugs.txt,
only partially fixed.

**Fixed:** `vomwonsq` — `DiscoverResources` propagates partial-failure errors
(4 callers made partial-tolerant); `discoverLoop` retries until clean success
(capped backoff, per-context singleflight); `watchCRDs` triggers debounced
re-discovery on runtime CRD changes.

### 6. No kubeconfig file watching

`LoadKubeconfigs` runs at startup and on explicit import only
(`internal/cluster/manager.go:121`, `internal/services/app.go:70`). When a cloud CLI
rotates certs/tokens in `~/.kube/config`, the app keeps stale credentials until manual
re-import. (Exec-plugin/OIDC token refresh *does* work via client-go's transport, so
this only bites for rotated CA/client certs written to disk.)

**Fixed:** `nxsqpwpu` — fsnotify watch over kubeconfig sources with 500ms debounce;
per-context credential-hash diff; changed + connected contexts get the
disconnect→connect→activate flow; `kubeconfigs:updated` refreshes the frontend.

---

## High — frontend staleness

### 7. Nothing re-lists after a reconnect

`ResourceStore` only refreshes on the `:resync` event — it never subscribes to
`status:{ctx}:connection` (`frontend/src/lib/stores/resource.svelte.ts:47-54`).
`clusterStore`'s status handler only acts when there is no active context; for the
currently-viewed cluster a disconnected→connected transition updates the indicator dot
but triggers no re-list, and `startNamespaceWatch` is idempotent by context name so it
won't restart (`frontend/src/lib/stores/cluster.svelte.ts:168-174, 263-288`).
Combined with #2, a recovered connection shows permanently stale data.

**Fixed:** `kkvkvvzk` (backend `ResyncContext` on recovery) + `zzxsoqvv` —
`ResourceStore` subscribes to connection status and re-lists on any
non-connected→connected transition (covers the re-import
disconnected→connecting→connected sequence).

### 8. Namespace watch (bcf6b3ed fix) has no resync recovery

The frontend subscribes to the namespace watch event but not its `:resync` variant
(`frontend/src/lib/stores/cluster.svelte.ts:270-287`). A 410 on that global watch
kills dropdown updates silently — the original bug returns after any long sleep.

**Fixed:** `zzxsoqvv` — namespace watch subscribes to its `:resync` event
(reload + re-watch).

### 9. `nsWatch` keyed by context name only

`frontend/src/lib/stores/cluster.svelte.ts:264` — re-importing a kubeconfig where the
same context name now points at a different cluster short-circuits the watch restart.

**Fixed:** `zzxsoqvv` — `startNamespaceWatch` gains a `force` parameter; the
status handler force-restarts the watch whenever the active context transitions
back to connected.

---

## Medium — streaming surfaces

### 10. Log streams die silently with no resume

When the upstream pod-logs stream drops mid-read, the goroutine logs a warning and
returns — no retry, no `SinceTime` resume, and the frontend receives `{type: eof}`,
indistinguishable from the pod finishing normally
(`internal/logs/streamer.go:143, 168-173, 237`). Follow mode doesn't survive container
restarts.

### 11. Log WebSocket write-error leaks a goroutine that blocks forever

On a WS write error, `HandleConn` returns early, skipping both the stream-map cleanup
and `cancel()` (`internal/logs/streamer.go:231-240`). The reader keeps running, fills
the 1024-item buffer, then blocks permanently on the channel send (`:163,183`).
Every dropped viewer socket leaks a goroutine plus an upstream connection.

### 12. Logs and exec hold the connection captured at session start

`internal/logs/streamer.go:129`, `internal/exec/manager.go:135` — with
`context.Background()`-rooted stream contexts. After a kubeconfig re-import, live
streams keep the stale client and are never torn down. (Port-forward and metrics
re-fetch the connection per iteration, which is correct.)

### 13. Exec gives no distinguishable "connection lost"

SPDY death is logged and the WS just closes (`internal/exec/manager.go:187-195`) —
the terminal can't tell an API outage from a normal shell exit, and there's no
reconnect offer.

### 14. Port-forward duplicate on `ReconnectSaved`

`StartForward` overwrites `m.forwards[ID]` without cancelling the prior entry
(`internal/portforward/manager.go:204`); if a selector forward's retry loop is still
alive when `ReconnectSaved` fires on reconnect, two tunnels race the same local port.

### 15. Frontend log viewers never reconnect

No `onclose` handler in `packages/ui/src/lib/LogViewer.svelte:84-113` (same class of
issue in `AggregateLogsPanel.svelte`, `LogsPanel.svelte`) — a streaming-server hiccup
or sleep stalls the view with zero feedback. `AggregateLogsPanel` also snapshots the
pod list once, so pods created by a rollout/scale-up are never streamed.

---

## Already handled correctly (verified — do not re-fix)

- Normal watch-channel closes re-watch from the current RV with 5s backoff and
  bookmark support (`internal/watcher/manager.go:205-211, 238-241, 258-264`).
- The `:resync` re-list genuinely rebuilds `items[]` and clears deleted resources —
  once, before the watch dies per #1 (`resource.svelte.ts:77-82`).
- `streaming:ready` startup race covered by the `GetStreamingConfig` pull fallback
  (`frontend/src/lib/stores/streaming.svelte.ts:14-28`).
- Port-forward drops emit `portforward:{ctx}:updated`, auto-restart with backoff for
  selector/statefulpod forwards; raw-pod forwards intentionally not retried.
- Exec-credential/OIDC token refresh works through client-go's cached transport.
- `ImportKubeconfigContent` disconnects+reconnects active contexts
  (`internal/services/cluster.go:276-309`) — but see #2 for what it misses.
- Watch grace-timer race guarded by the `graceTimer != nil` check
  (`internal/watcher/manager.go:298`).
- Health-monitor goroutines torn down via context on Deactivate/Disconnect; recovery
  re-emits StatusConnected and re-runs discovery (`internal/cluster/manager.go:715-724`).

---

## Root-cause summary

Findings #1, #2, and #7 share one root cause: a **lifecycle ownership gap**. Watches
are keyed in a map that outlives their goroutines, and the goroutines outlive their
connection. client-go's Reflector/Informer machinery solves this whole class
(ListAndWatch with automatic 410 re-list and connection-aware backoff); the hand-rolled
`runWatch` reimplements ~70% of it, and the missing 30% is where these bugs live.

Finding #3 is the nastiest failure mode because it disables the *recovery system
itself* — every "reconnect on failure" feature depends on the health probe being able
to fail fast. A `rest.Config.Timeout` of ~10–15s is the single highest-leverage
one-line fix in this document.
