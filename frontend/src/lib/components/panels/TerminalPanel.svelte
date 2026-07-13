<script lang="ts">
  import {onDestroy} from "svelte";
  import {OpenExecSession, CloseExecSession} from "$api/github.com/Vilsol/klados/internal/services/execservice.js";
  import {GetResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {streamingStore} from "$lib/stores/streaming.svelte";
  import {sessionStore} from "$lib/stores/session.svelte";
  import {bottomPanelStore, type PanelKind} from "$lib/stores/bottom-panel.svelte";
  import {volumeBrowserStore} from "$lib/stores/volumeBrowser.svelte";
  import {resourceCache} from "$lib/stores/resourceCache.svelte";
  import {Terminal, Combobox} from "@klados/ui";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    ctxName,
    namespace,
    name,
    tabId,
    tabKind = "terminal",
    managedId,
    initialContainer = "",
  }: {
    obj: Record<string, KubernetesResource>;
    ctxName: string;
    namespace: string;
    name: string;
    tabId?: string;
    tabKind?: PanelKind;
    managedId?: string;
    initialContainer?: string;
  } = $props();

  // ---- terminal-pending (volume browser) lifecycle ----
  const STUCK_REASONS = new Set([
    "ImagePullBackOff",
    "ErrImagePull",
    "CreateContainerConfigError",
  ]);
  const STUCK_TIMEOUT_MS = 60_000;

  let pendingStart = $state(Date.now());
  let nowTs = $state(Date.now());

  // Tick every second while pending so the stuck-timeout derivation stays fresh.
  $effect(() => {
    if (tabKind !== "terminal-pending") return;
    const h = setInterval(() => (nowTs = Date.now()), 1000);
    return () => clearInterval(h);
  });

  $effect(() => {
    if (tabKind === "terminal-pending") {
      pendingStart = Date.now();
    }
  });

  interface ContainerState {
    waiting?: {reason?: string; message?: string};
    running?: {startedAt?: string};
    terminated?: {reason?: string};
  }
  interface ContainerStatus {
    ready?: boolean;
    state?: ContainerState;
  }
  interface PodStatus {
    phase?: string;
    containerStatuses?: ContainerStatus[];
  }
  interface Pod {
    metadata?: {name?: string; namespace?: string};
    status?: PodStatus;
  }

  // During terminal-pending we poll GetResource directly (every 2s) so we don't
  // depend on a ResourceStore being active for pods in this namespace. The cache
  // is used as a fallback when it happens to be populated (e.g. user is also
  // viewing the Pods list).
  let polledPod = $state<Pod | undefined>(undefined);

  $effect(() => {
    if (tabKind !== "terminal-pending") return;
    const _ctx = ctxName;
    const _ns = namespace;
    const _name = name;
    let cancelled = false;

    const fetchOnce = async () => {
      try {
        const obj = (await GetResource(_ctx, "core.v1.pods", _ns, _name)) as Pod | undefined;
        if (!cancelled && obj) {
          polledPod = obj;
          if (tabId) {
            bottomPanelStore.setObj(tabId, obj as unknown as Record<string, unknown>);
          }
        }
      } catch {
        // transient — next tick may succeed
      }
    };
    void fetchOnce();
    const h = setInterval(fetchOnce, 2000);
    return () => {
      cancelled = true;
      clearInterval(h);
    };
  });

  const cachedPod = $derived<Pod | undefined>(
    tabKind === "terminal-pending"
      ? (resourceCache.findByNamespaceName(ctxName, "core.v1.pods", namespace, name) as
          | Pod
          | undefined)
      : undefined,
  );

  // Prefer the cache (live watch updates) when present; otherwise use the polled result.
  const watchedPod = $derived<Pod | undefined>(cachedPod ?? polledPod);

  const phase = $derived<string>(watchedPod?.status?.phase ?? "Pending");
  const containerStatuses = $derived<ContainerStatus[]>(watchedPod?.status?.containerStatuses ?? []);
  const firstState = $derived<ContainerState>(containerStatuses[0]?.state ?? {});
  const firstReady = $derived<boolean>(!!containerStatuses[0]?.ready);

  const waitingReason = $derived<string | undefined>(firstState?.waiting?.reason);
  const waitingMessage = $derived<string | undefined>(firstState?.waiting?.message);
  const containerStateLabel = $derived<string>(
    firstState?.waiting
      ? `Waiting (${firstState.waiting.reason ?? "unknown"})`
      : firstState?.running
        ? "Running"
        : firstState?.terminated
          ? `Terminated (${firstState.terminated.reason ?? "unknown"})`
          : "Unknown",
  );

  const isStuck = $derived<boolean>(
    tabKind === "terminal-pending" &&
      phase === "Pending" &&
      !!waitingReason &&
      STUCK_REASONS.has(waitingReason) &&
      nowTs - pendingStart > STUCK_TIMEOUT_MS,
  );

  // Transition terminal-pending → terminal when the pod is actually ready.
  let transitioned = $state(false);
  $effect(() => {
    if (
      !transitioned &&
      tabKind === "terminal-pending" &&
      tabId &&
      phase === "Running" &&
      firstReady
    ) {
      transitioned = true;
      bottomPanelStore.setKind(tabId, "terminal");
    }
  });

  function deletePending() {
    // closeTab handles Stop teardown for managed terminal tabs — avoid double-Stop.
    if (tabId) {
      bottomPanelStore.closeTab(tabId);
    }
  }

  async function deleteAndRetry() {
    if (!managedId) return;
    await volumeBrowserStore.retry(managedId);
  }

  interface TermSession {
    id: string;
    container: string;
    shell: string;
    clearFn?: () => void;
  }

  const containers = $derived<KubernetesResource[]>([
    ...(obj.spec?.containers ?? []).map((c: KubernetesResource) => ({name: c.name, init: false})),
    ...(obj.spec?.initContainers ?? []).map((c: KubernetesResource) => ({name: c.name, init: true})),
  ]);

  const shells = ["bash", "sh", "zsh"];

  let selectedContainer = $state("");

  $effect(() => {
    if (initialContainer) {
      selectedContainer = initialContainer;
    }
  });
  let selectedShell = $state("bash");
  let sessions = $state<TermSession[]>([]);
  let activeIdx = $state(0);
  let error = $state<string | null>(null);
  let loading = $state(false);

  $effect(() => {
    if (containers.length > 0 && !selectedContainer) {
      selectedContainer = containers[0].name;
    }
  });

  // Close all sessions when the target pod changes
  $effect(() => {
    const _ctx = ctxName;
    const _ns = namespace;
    const _name = name;
    return () => {
      for (const s of sessions) {
        CloseExecSession(s.id);
      }
      sessions = [];
      activeIdx = 0;
      error = null;
      selectedContainer = "";
    };
  });

  const containerOptions = $derived(
    containers.map((c) => ({
      value: c.name,
      label: c.init ? `${c.name} (init)` : c.name,
    })),
  );

  async function connect() {
    error = null;
    loading = true;
    try {
      const id = await OpenExecSession(ctxName, namespace, name, selectedContainer, selectedShell);
      sessions = [...sessions, {id, container: selectedContainer, shell: selectedShell}];
      activeIdx = sessions.length - 1;
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  function removeSession(i: number) {
    if (i < 0 || i >= sessions.length) {
      return;
    }
    removeSessionById(sessions[i].id);
  }

  function removeSessionById(id: string) {
    const idx = sessions.findIndex((s) => s.id === id);
    if (idx < 0) {
      return;
    }
    CloseExecSession(id);
    sessions = sessions.filter((s) => s.id !== id);
    if (activeIdx >= sessions.length) {
      activeIdx = Math.max(0, sessions.length - 1);
    }
  }

  onDestroy(() => {
    for (const s of sessions) {
      CloseExecSession(s.id);
    }
  });
</script>

{#if tabKind === "terminal-pending"}
  <div class="flex flex-col gap-4 p-4 overflow-auto h-full">
    {#if isStuck}
      <div class="flex flex-col gap-3 border border-destructive/50 bg-destructive/5 rounded p-3">
        <div class="text-sm font-medium text-destructive" data-testid="pending-error">
          Pod {name} failed to start: {waitingReason}
        </div>
        {#if waitingMessage}
          <pre class="text-xs text-muted whitespace-pre-wrap break-words">{waitingMessage}</pre>
        {/if}
        <div class="flex gap-2">
          <button
            type="button"
            onclick={deleteAndRetry}
            class="px-3 py-1.5 text-sm bg-accent text-white rounded hover:opacity-90 transition-opacity"
          >
            Delete &amp; Retry
          </button>
          <button
            type="button"
            onclick={deletePending}
            class="px-3 py-1.5 text-sm border border-border rounded hover:bg-surface-hover transition-colors"
          >
            Delete
          </button>
        </div>
      </div>
    {:else}
      <div class="flex items-center gap-3" data-testid="pending-waiting">
        <div
          class="w-4 h-4 border-2 border-accent border-t-transparent rounded-full animate-spin"
          aria-hidden="true"
        ></div>
        <span class="text-sm text-muted">
          Waiting for pod {name}: {phase}/{containerStateLabel}…
        </span>
      </div>
    {/if}
  </div>
{:else if sessions.length > 0 && streamingStore.config}
  <div class="flex flex-col h-full overflow-hidden">
    <!-- Compact header: selectors + tab bar + new button -->
    <div class="flex items-center gap-2 px-2 py-1 border-b border-border bg-surface shrink-0 text-xs flex-wrap">
      <div class="w-36"><Combobox bind:value={selectedContainer} options={containerOptions} placeholder="Container" size="xs" /></div>

      <!-- Shell selector (compact) -->
      <div class="flex gap-1">
        {#each shells as shell}
          <button
            type="button"
            onclick={() => selectedShell = shell}
            class="px-2 py-0.5 text-xs rounded border transition-colors
              {selectedShell === shell
                ? 'border-accent text-accent bg-accent/10'
                : 'border-border text-muted hover:bg-surface-hover'}"
          >
            {shell}
          </button>
        {/each}
      </div>

      <!-- Session tabs -->
      <div class="flex items-center gap-1 flex-1 overflow-x-auto">
        {#each sessions as s, i}
          <button
            type="button"
            onclick={() => activeIdx = i}
            class="flex items-center gap-1 px-2 py-0.5 rounded border whitespace-nowrap transition-colors
              {i === activeIdx
                ? 'border-accent text-accent bg-accent/10'
                : 'border-border text-muted hover:bg-surface-hover'}"
          >
            <span>{s.shell}:{s.container}</span>
            <span
              role="button"
              tabindex="0"
              aria-label="Remove session"
              onclick={(e) => { e.stopPropagation(); removeSession(i) }}
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.stopPropagation(); removeSession(i) } }}
              class="ml-1 hover:text-destructive"
              >×</span
            >
          </button>
        {/each}
      </div>

      <button
        type="button"
        onclick={connect}
        disabled={loading || !selectedContainer}
        class="shrink-0 px-2 py-0.5 text-xs border border-border rounded hover:bg-surface-hover disabled:opacity-50 transition-colors"
        title="New session"
        aria-label="New terminal session"
      >
        +
      </button>

      <div class="flex items-center gap-1 ml-auto shrink-0">
        <button
          type="button"
          onclick={() => sessions[activeIdx]?.clearFn?.()}
          disabled={sessions.length === 0}
          class="px-2 py-0.5 text-xs border border-border rounded hover:bg-surface-hover disabled:opacity-50 transition-colors text-muted"
          title="Clear terminal"
        >
          Clear
        </button>
        <button
          type="button"
          onclick={() => sessionStore.terminalFontSize = Math.max(8, sessionStore.terminalFontSize - 1)}
          class="text-xs text-muted hover:text-fg border border-border rounded px-1.5 py-0.5 transition-colors"
          title="Decrease font size"
        >
          −
        </button>
        <span class="text-xs text-muted w-6 text-center">{sessionStore.terminalFontSize}</span>
        <button
          type="button"
          onclick={() => sessionStore.terminalFontSize = Math.min(24, sessionStore.terminalFontSize + 1)}
          class="text-xs text-muted hover:text-fg border border-border rounded px-1.5 py-0.5 transition-colors"
          title="Increase font size"
        >
          +
        </button>
      </div>
    </div>

    <!-- Terminal layers: all mounted, active one visible -->
    <div class="relative flex-1 overflow-hidden">
      {#each sessions as session, i}
        <div class="absolute inset-0" class:invisible={i !== activeIdx}>
          <Terminal
            sessionID={session.id}
            streamingConfig={streamingStore.config}
            fontSize={sessionStore.terminalFontSize}
            onclear={(fn) => { sessions[i] = { ...sessions[i], clearFn: fn } }}
            ondisconnect={() => removeSessionById(session.id)}
          />
        </div>
      {/each}
    </div>
  </div>
{:else}
  <div class="flex flex-col gap-4 p-4 overflow-auto">
    {#if !streamingStore.config}
      <p class="text-sm text-muted">Waiting for streaming server…</p>
    {:else}
      <!-- Container selector -->
      <div class="flex flex-col gap-1">
        <!-- svelte-ignore a11y_label_has_associated_control -->
        <label class="text-xs font-medium text-muted uppercase tracking-wide">Container</label>
        <Combobox bind:value={selectedContainer} options={containerOptions} placeholder="Select container" />
      </div>

      <!-- Shell selector -->
      <div class="flex flex-col gap-1">
        <!-- svelte-ignore a11y_label_has_associated_control -->
        <label class="text-xs font-medium text-muted uppercase tracking-wide">Shell</label>
        <div class="flex gap-2">
          {#each shells as shell}
            <button
              type="button"
              onclick={() => selectedShell = shell}
              class="px-3 py-1.5 text-sm rounded border transition-colors
                {selectedShell === shell
                  ? 'border-accent text-accent bg-accent/10'
                  : 'border-border text-muted hover:bg-surface-hover'}"
            >
              {shell}
            </button>
          {/each}
        </div>
      </div>

      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}

      <button
        type="button"
        onclick={connect}
        disabled={loading || !selectedContainer}
        class="self-start px-4 py-2 text-sm bg-accent text-white rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
      >
        {loading ? 'Connecting…' : 'Connect'}
      </button>
    {/if}
  </div>
{/if}
