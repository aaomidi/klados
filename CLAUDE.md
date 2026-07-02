# OpenWolf

@.wolf/OPENWOLF.md

This project uses OpenWolf for context management. Read and follow .wolf/OPENWOLF.md every session. Check .wolf/cerebrum.md before generating code. Check .wolf/anatomy.md before reading files.


# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Module

`github.com/Vilsol/klados` — Go 1.26. Transport is ConnectRPC + a server-streamed event hub (see `proto/klados/v1/`, `internal/server/`); Wails v3 remains only as the desktop window shell.

## Monorepo

pnpm workspace (`pnpm-workspace.yaml`): `frontend/`, `packages/*`, `apps/*`. Use `pnpm install` (not npm).

Tool versions managed by `mise.toml` (wails3, go-jsonschema, node 25, tinygo, pnpm).

## Build

```bash
# Dev mode (hot reload, starts Vite + Go watcher)
task dev

# Frontend only
cd frontend && pnpm install && pnpm build

# Desktop binary (requires CGO for Wails/GTK on Linux)
go build .

# Headless server binary (no CGO/Wails — what the Docker image builds)
go build -tags headless .

# Run the self-hosted web server (serves the embedded SPA + ConnectRPC API)
./klados serve --addr :8080

# Container image + Kubernetes manifests
docker build -t klados .        # see deploy/kubernetes/klados.yaml

# Generate ConnectRPC code (required after clone and after editing
# proto/klados/v1/*.proto; outputs gen/ and frontend/src/gen/, both gitignored):
buf generate

# Regenerate plugin types from JSON Schemas:
mise run generate:plugin-types

# Type-check frontend:
cd frontend && pnpm check
```

## Test

```bash
# Go — packages that don't need CGO (fast)
go test ./internal/config/ ./internal/session/ ./internal/cluster/ ./internal/server/ ./internal/watcher/ -v

# Go — all packages
go test ./internal/... -v

# Single Go test
go test ./internal/cluster/ -run TestLoadKubeconfigs -v

# Integration tests (requires a live cluster)
go test ./internal/... -v -tags integration

# Frontend
cd frontend && pnpm test

# Single frontend test file
cd frontend && npx vitest run src/lib/__tests__/Header.svelte.test.ts
```

## Architecture

### Data flow

```
Kubernetes API → cluster.Manager (dynamic client)
                      ↓
              resource.ResourceEngine  →  List/Get/Delete via dynamic.Interface
              resource.EnricherRegistry →  per-GVR Enricher injects computed fields
                      ↓
              watcher.WatchManager    →  emits watch:{ctx}:{gvr}:{ns} into server.Hub
                      ↓
              server.Hub              →  coalesces bursts (~75ms), fans out to
                                          Connect server-streams (EventService.Subscribe)
              services.*              →  RPC layer, adapted by internal/server handlers
                                          (ConnectRPC over HTTP/2 h2c)
                      ↓
              frontend ResourceStore  →  subscribes via the @wailsio/runtime shim
                                          (Vite alias → src/lib/transport/), owns items[]
                      ↓
              ResourceList.svelte     →  TanStack Virtual, CEL column rendering
```

Both deployment shapes run this same stack: `klados serve` binds it publicly
(Kubernetes/remote, port-forwarding disabled via GetCapabilities), the desktop
shell (`cmd/desktop.go`, default build) runs it on loopback and points a Wails
webview at it. Logs/exec ride plain WebSockets (`/ws/logs/{id}`,
`/ws/exec/{id}`) because exec needs full duplex, which browser fetch cannot do.

Desktop-only native features (file dialogs, pop-out OS windows) are provided
by a `server.Desktop` interface the Wails shell implements; server mode passes
nil and the handlers return `CodeUnimplemented`, which the frontend catches to
fall back to web equivalents (file inputs, `window.open`). `GetCapabilities`
(`capabilitiesStore`) lets the UI hide features up front. `task dev` still hot-
reloads: `wails3 dev` exports `FRONTEND_DEVSERVER_URL`, and the server reverse-
proxies non-API requests (including Vite's HMR websocket) to it, keeping the
API same-origin.

### Go backend (`internal/`)

| Package | Responsibility |
|---|---|
| `logging/` | `Setup()` returns `context.Context` with tint-backed slog logger via slox. Use `slox.Info(ctx, ...)` everywhere — never pass `*slog.Logger` directly. |
| `config/` | JSON config at `$XDG_CONFIG_HOME/klados/config.json` |
| `session/` | State at `$XDG_STATE_HOME/klados/session.json`, debounced 500ms save |
| `cluster/` | `Manager`: kubeconfig loading, connect/disconnect, health monitor (15s), `DiscoverResources()` emits `discovery:{ctx}:resources` on connect |
| `resource/` | `Registry` (CEL-validated descriptors), `ResourceEngine` (List/Get/Delete), `EnricherRegistry` + per-resource enrichers that inject display fields into unstructured objects |
| `watcher/` | `WatchManager`: start/stop per `(ctx, gvr, namespace)` key; 30s grace period before actually stopping; emits `watch:{ctx}:{gvr}:{ns}` events with `{type, object}` payload (managedFields stripped) |
| `server/` | The transport: `Hub` (coalescing event bus), ConnectRPC handlers adapting `services/`, WebSocket routes for logs/exec, plugin static serving, SPA serving, `Bootstrap()` service wiring shared by `serve` and desktop |
| `logs/` | `LogStreamer`: per-container log streaming over WebSocket, 1024-item buffered channel for backpressure |
| `exec/` | `ExecManager`: interactive shell sessions via WebSocket, resize via text JSON frames |
| `portforward/` | `Manager`: port-forward lifecycle, emits `portforward:{ctx}:{id}` (per-forward) and `portforward:{ctx}:updated` (aggregate) events |
| `metrics/` | Metrics collection and aggregation |
| `services/` | Transport-agnostic service layer (`Startup(ctx)`/`Shutdown()` lifecycle, injected `emit`/`on`) — `AppService` owns `cluster.Manager`; `ResourceService` owns `ResourceEngine` and `WatchManager` |
| `plugin/` | Plugin system: wazero Wasm runtime, manifest validation, permission enforcement, enricher adapter, hot reload via fsnotify. See [PLUGIN_ARCHITECTURE.md](PLUGIN_ARCHITECTURE.md) for full spec. |

### GVR format

Dot-separated: `apps.v1.deployments`, `core.v1.pods`, `networking.k8s.io.v1.ingresses`. The `core` prefix replaces empty group. `ParseGVR()` splits from the right (handles groups with dots).

### Three-stage rendering pipeline

1. **Go enricher** — injects computed fields into `unstructured.Unstructured` (e.g. `status.readyDisplay`, `status.restartCount`)
2. **CEL extraction** — column `expr` strings evaluated at render time via `cel-js` `evalExpr(expr, obj)`
3. **Frontend renderer** — `renderType`: `text` | `badge` | `age` | `progress`

Adding a new resource type requires: a `Descriptor` in `internal/resource/builtin.go` (optional enricher), and the descriptor is automatically serialized to the frontend via `GetDescriptors()`. Unknown GVRs get a fallback descriptor (Name, Namespace, Age).

### Events

Frontend code still imports `Events` from `@wailsio/runtime`, but a Vite alias resolves that to `src/lib/transport/wails-runtime.ts`, which backs `Events.On(topic)` with one Connect server-stream per subscription (auto-reconnect with backoff) and `Events.Emit` with `EventService.Publish`. Callbacks receive `{ name, data }` — always unwrap with `wailsEvent.data`. `Events.On()` returns an unsubscribe function.

### Bindings facade

`frontend/bindings/.../services/*.js` keep their original module paths and export names but are hand-written facades over the generated connect-web clients (`frontend/src/gen/`, from `buf generate`). Structured results ride as `bytes *_json` proto fields decoded with `fromJsonBytes` — byte-identical to the old Wails JSON shapes. After changing a proto, run `buf generate`; after changing a service signature, update the proto + handler + facade.

### Frontend (`frontend/src/`)

| Path | Responsibility |
|---|---|
| `lib/stores/cluster.svelte.ts` | `clusterStore` singleton: contexts, `activeContext`, `selectedNamespaces[]` (empty = all namespaces), namespace list |
| `lib/stores/resource.svelte.ts` | `ResourceStore` (created per page): owns watch lifecycle, holds `items[]` |
| `lib/stores/session.svelte.ts` | Sidebar collapsed state, tab list |
| `lib/stores/notification.svelte.ts` | Toast queue (5s auto-dismiss) |
| `lib/registry/index.ts` | `DescriptorRegistry`: loads from Go via `GetDescriptors()`, provides `get(gvr)` with fallback; `evalExpr()` for CEL |
| `lib/registry/loaded.svelte.ts` | Reactive signal (`registryLoaded()`) — gate descriptor lookups behind this |
| `routes/routes.ts` | `/ → ClusterList`, `/c/:ctx → ClusterOverview`, `/c/:ctx/:gvr → ResourceListPage`, `/c/:ctx/:gvr/:ns/:name → ResourceDetailPage` |

### Namespace selection

`clusterStore.selectedNamespaces: string[]` — empty means all namespaces. `ResourceListPage` passes `watchNamespace = selectedNamespaces.length === 1 ? selectedNamespaces[0] : ''` to the watch (empty = all). Multi-select filters client-side in `ResourceList`.

## VCS

This repo uses plain git. At the end of every unit of work (bugfix, feature, small change, etc.), create a logical git commit. Never leave work uncommitted.

## Conventions

- **Svelte 5 runes** (`$state`, `$derived`, `$effect`, `$props`) — class-based stores exported as singletons. Never use `.svelte.ts` extension for non-reactive files (the Vite Svelte plugin treats any `.svelte.*` import as a component).
- **Tailwind v4** custom tokens: `bg`, `fg`, `muted`, `border`, `accent`, `surface`, `surface-hover`, `destructive`. Dark mode via `.dark` on `<html>`.
- **Logging**: `slox.Info/Warn/Error(ctx, msg, key, val...)` — context carries the logger. Structs store `ctx context.Context` (not `*slog.Logger`).
- **Tests**: `testza` for Go assertions, `vitest` + `@testing-library/svelte` for frontend. Frontend tests mock `@wailsio/runtime` (in `setup.ts`) and must also mock any binding they import transitively.
- **Wails bindings**: TypeScript files at `frontend/bindings/...` — import with `.js` extension (ESM pattern). Regenerate after any Go service change.
