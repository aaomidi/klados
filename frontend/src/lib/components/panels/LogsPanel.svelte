<script lang="ts">
  import {onDestroy, untrack} from "svelte";
  import {StartLogStream, StopLogStream} from "$api/github.com/Vilsol/klados/internal/services/logservice.js";
  import {LogOptions} from "$api/github.com/Vilsol/klados/internal/logs/models.js";
  import {streamingStore} from "$lib/stores/streaming.svelte";
  import {sessionStore} from "$lib/stores/session.svelte";
  import {LogViewer, Combobox} from "@klados/ui";
  import {getLogger} from "$lib/logger";
  import type {KubernetesResource} from "$lib/types";

  const log = getLogger("logs");

  let {
    obj,
    ctxName,
    namespace,
    name,
    initialContainer = "",
  }: {
    obj: Record<string, KubernetesResource>;
    ctxName: string;
    namespace: string;
    name: string;
    initialContainer?: string;
  } = $props();

  const containers = $derived<KubernetesResource[]>([
    ...(obj.spec?.containers ?? []).map((c: KubernetesResource) => ({name: c.name, init: false})),
    ...(obj.spec?.initContainers ?? []).map((c: KubernetesResource) => ({name: c.name, init: true})),
  ]);

  let selectedContainer = $state("");

  $effect(() => {
    if (initialContainer) {
      selectedContainer = initialContainer;
    }
  });
  let timestamps = $state(false);
  let previous = $state(false);

  const showTimestamps = $derived(timestamps);
  let streamID = $state<string | null>(null);
  let starting = $state(false);
  let logViewer = $state<ReturnType<typeof LogViewer>>();
  let downloadDropdownOpen = $state(false);

  // Validated container: always valid for the current pod's container list
  const effectContainer = $derived(containers.some((c) => c.name === selectedContainer) ? selectedContainer : (containers[0]?.name ?? ""));

  // Reactive readiness: true once the target container has produced (or is producing) logs.
  // - previous=false: container must be running or terminated at least once
  // - previous=true:  lastState.terminated must exist
  // "" (All) falls through to ready so the multi-container path still works.
  const containerReady = $derived.by(() => {
    if (!effectContainer) return true;
    const statuses = [
      ...(obj.status?.containerStatuses ?? []),
      ...(obj.status?.initContainerStatuses ?? []),
    ] as KubernetesResource[];
    const cs = statuses.find((s) => s.name === effectContainer);
    if (!cs) return false;
    if (previous) return !!cs.lastState?.terminated;
    return !!(cs.state?.running || cs.state?.terminated || cs.lastState?.terminated);
  });

  let tailLines = $state<number | undefined>(200);
  let scrollToTopOnLoad = $state(false);

  // Keep selectedContainer UI state in sync when pod changes; reset load options
  $effect(() => {
    const c = effectContainer;
    untrack(() => {
      selectedContainer = c;
      tailLines = 200;
      scrollToTopOnLoad = false;
    });
  });

  $effect(() => {
    const container = effectContainer;
    const prev = previous;
    const _ctx = ctxName;
    const _ns = namespace;
    const _name = name;
    const _tail = tailLines;
    const ready = containerReady;
    if (!(container && streamingStore.config && ready)) {
      return;
    }

    let cancelled = false;
    let myID: string | null = null;
    starting = true;

    StartLogStream(
      _ctx,
      _ns,
      _name,
      new LogOptions({
        container,
        follow: true,
        tailLines: _tail,
        timestamps: true,
        previous: prev,
      }),
    )
      .then((id) => {
        if (cancelled) {
          StopLogStream(id);
          return;
        }
        myID = id;
        streamID = id;
        starting = false;
      })
      .catch((e) => {
        log.warn("Log stream start failed", {error: String(e)});
        starting = false;
      });

    return () => {
      cancelled = true;
      starting = false;
      streamID = null;
      if (myID) {
        StopLogStream(myID);
      }
    };
  });

  function handleClickOutside(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (!t.closest("[data-download-dropdown]")) {
      downloadDropdownOpen = false;
    }
  }

  const filename = $derived(`${namespace}-${name}-${selectedContainer || "all"}`);

  let downloading = $state(false);

  async function downloadAll() {
    if (downloading || !streamingStore.config) {
      return;
    }
    downloading = true;
    try {
      const id = await StartLogStream(
        ctxName,
        namespace,
        name,
        new LogOptions({
          container: selectedContainer,
          follow: false,
          timestamps: true,
        }),
      );
      const allLines: string[] = [];
      let buf = "";
      await new Promise<void>((resolve) => {
        const socket = new WebSocket(streamingStore.logStreamUrl(id));
        socket.onmessage = (e) => {
          if (typeof e.data !== "string") {
            return;
          }
          try {
            const msg = JSON.parse(e.data);
            if (msg.type === "eof" || msg.type === "error") {
              socket.close();
              resolve();
              return;
            }
          } catch {
            /* empty */
          }
          const parts = (buf + e.data).split("\n");
          buf = parts.pop() ?? "";
          allLines.push(...parts);
        };
        socket.onerror = () => resolve();
        socket.onclose = () => resolve();
      });
      if (buf) {
        allLines.push(buf);
      }
      const blob = new Blob([allLines.join("\n")], {type: "text/plain"});
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = `${filename}.log`;
      a.click();
      URL.revokeObjectURL(a.href);
      StopLogStream(id);
    } finally {
      downloading = false;
    }
  }

  const containerOptions = $derived([
    {value: "", label: "All"},
    ...containers.map((c) => ({
      value: c.name,
      label: c.init ? `${c.name} (init)` : c.name,
    })),
  ]);

  onDestroy(() => {
    if (streamID) {
      StopLogStream(streamID);
    }
  });
</script>

<svelte:document onclick={handleClickOutside} />

{#if !streamingStore.config}
  <div class="flex items-center justify-center h-full text-sm text-muted">Waiting for streaming server…</div>
{:else}
  <div class="flex flex-col h-full overflow-hidden">
    <!-- Control bar -->
    <div class="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-surface shrink-0 text-xs flex-wrap">
      <div class="w-36"><Combobox bind:value={selectedContainer} options={containerOptions} placeholder="All" size="xs" /></div>

      <label class="flex items-center gap-1 text-xs text-muted select-none cursor-pointer">
        <input type="checkbox" bind:checked={timestamps} class="accent-accent">
        Timestamps
      </label>
      <label class="flex items-center gap-1 text-xs text-muted select-none cursor-pointer">
        <input type="checkbox" bind:checked={previous} class="accent-accent">
        Previous
      </label>

      <button
        type="button"
        onclick={() => { tailLines = undefined; scrollToTopOnLoad = true }}
        class="text-xs text-muted hover:text-fg border border-border rounded px-2 py-1 transition-colors"
        title="Load full history and jump to beginning"
      >
        Full history
      </button>

      <div class="flex items-center gap-1">
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

      <div class="relative ml-auto" data-download-dropdown>
        <button
          type="button"
          onclick={() => (downloadDropdownOpen = !downloadDropdownOpen)}
          class="flex items-center gap-1 text-xs text-muted hover:text-fg border border-border rounded px-2 py-1 transition-colors"
        >
          Download ↓
        </button>
        {#if downloadDropdownOpen}
          <div class="absolute top-full right-0 mt-1 min-w-[5rem] rounded border border-border bg-bg shadow-lg z-50">
            <button
              type="button"
              onclick={() => { logViewer?.downloadVisible(); downloadDropdownOpen = false }}
              class="w-full text-left px-3 py-1.5 text-xs text-muted hover:bg-surface-hover transition-colors"
            >
              Visible
            </button>
            <button
              type="button"
              onclick={() => { downloadAll(); downloadDropdownOpen = false }}
              class="w-full text-left px-3 py-1.5 text-xs text-muted hover:bg-surface-hover transition-colors"
            >
              All
            </button>
          </div>
        {/if}
      </div>

      {#if starting}
        <span class="text-xs text-muted italic">Connecting…</span>
      {/if}
    </div>

    <!-- Log viewer -->
    <div class="flex-1 overflow-hidden">
      {#if streamID}
        <LogViewer
          bind:this={logViewer}
          {streamID}
          streamingConfig={streamingStore.config}
          {showTimestamps}
          {filename}
          {scrollToTopOnLoad}
          fontSize={sessionStore.terminalFontSize}
        />
      {:else if !containerReady}
        <div class="flex items-center justify-center h-full text-sm text-muted">
          {previous ? "No previous logs available — container has not terminated yet." : "Waiting for container to start…"}
        </div>
      {:else}
        <div class="flex items-center justify-center h-full text-sm text-muted">Loading…</div>
      {/if}
    </div>
  </div>
{/if}
