<script lang="ts">
  import {onDestroy, untrack} from "svelte";
  import {ListResources} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {StartLogStream, StopLogStream} from "$api/github.com/Vilsol/klados/internal/services/logservice.js";
  import {LogOptions} from "$api/github.com/Vilsol/klados/internal/logs/models.js";
  import {streamingStore} from "$lib/stores/streaming.svelte";
  import {sessionStore} from "$lib/stores/session.svelte";
  import {AggregateLogStore} from "$lib/stores/aggregate-logs.svelte";
  import {createVirtualizer} from "@tanstack/svelte-virtual";
  import type {SvelteVirtualizer} from "@tanstack/svelte-virtual";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    ctxName,
    namespace,
  }: {
    obj: Record<string, KubernetesResource>;
    ctxName: string;
    namespace: string;
    name: string;
  } = $props();

  const store = new AggregateLogStore();
  let streamIds: string[] = [];
  let loading = $state(true);
  let error = $state<string | null>(null);
  let podCount = $state(0);

  let scrollEl = $state<HTMLDivElement | undefined>(undefined);
  let sticky = $state(true);
  let programmaticScroll = false;

  const virtualizerStore = createVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: 0,
    getScrollElement: () => scrollEl ?? null,
    estimateSize: () => 20,
    overscan: 15,
  });

  let virt: SvelteVirtualizer<HTMLDivElement, HTMLDivElement>;
  virtualizerStore.subscribe((v) => {
    virt = v;
  });

  $effect(() => {
    const count = store.lines.length;
    untrack(() => {
      virt?.setOptions({
        count,
        getScrollElement: () => scrollEl ?? null,
        estimateSize: () => 20,
        overscan: 15,
      });
      if (sticky && scrollEl) {
        programmaticScroll = true;
        requestAnimationFrame(() => {
          if (scrollEl) {
            scrollEl.scrollTop = scrollEl.scrollHeight;
          }
        });
      }
    });
  });

  function onScroll(e: Event) {
    if (programmaticScroll) {
      programmaticScroll = false;
      return;
    }
    const el = e.target as HTMLElement;
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
    sticky = dist < 40;
  }

  async function startStreams() {
    if (!streamingStore.config) {
      return;
    }
    loading = true;
    error = null;
    try {
      const pods = await ListResources(ctxName, "core.v1.pods", namespace);
      const selector: Record<string, string> = obj.spec?.selector?.matchLabels ?? {};
      const matched = (pods ?? []).filter((pod: KubernetesResource) =>
        Object.entries(selector).every(([k, v]) => pod.metadata?.labels?.[k] === v),
      );

      podCount = matched.length;
      if (matched.length === 0) {
        error = "No matching pods found";
        loading = false;
        return;
      }

      for (const pod of matched) {
        const podName: string = pod.metadata?.name ?? "";
        if (!podName) {
          continue;
        }

        // biome-ignore lint/performance/noAwaitInLoops: streams must start sequentially to track IDs per pod
        const id = await StartLogStream(
          ctxName,
          namespace,
          podName,
          new LogOptions({
            follow: true,
            tailLines: 200,
            timestamps: false,
          }),
        );
        streamIds.push(id);

        const ws = new WebSocket(streamingStore.logStreamUrl(id));

        store.addStream(podName, id, ws);

        let buf = "";
        ws.onmessage = (e) => {
          if (typeof e.data !== "string") {
            return;
          }
          try {
            const msg = JSON.parse(e.data);
            if (msg.type === "eof" || msg.type === "error") {
              store.markEnded(podName);
              return;
            }
          } catch {
            /* empty */
          }
          const parts = (buf + e.data).split("\n");
          buf = parts.pop() ?? "";
          for (const line of parts) {
            if (line) {
              store.appendLine(podName, line);
            }
          }
        };
        ws.onclose = () => {
          if (buf) {
            store.appendLine(podName, buf);
            buf = "";
          }
          store.markEnded(podName);
        };
        ws.onerror = () => store.markEnded(podName);
      }
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (streamingStore.config) {
      untrack(() => startStreams());
    }
  });

  onDestroy(() => {
    for (const id of streamIds) {
      StopLogStream(id);
    }
    store.destroy();
  });
</script>

{#if !streamingStore.config}
  <div class="flex items-center justify-center h-full text-sm text-muted">Waiting for streaming server…</div>
{:else}
  <div class="flex flex-col h-full overflow-hidden" style="--log-font-size: {sessionStore.terminalFontSize}px">
    <!-- Control bar -->
    <div class="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-surface shrink-0 text-xs flex-wrap">
      <span class="text-muted">
        {#if loading}
          Connecting…
        {:else if error}
          <span class="text-destructive">{error}</span>
        {:else}
          {podCount}
          pod{podCount === 1 ? '' : 's'}
        {/if}
      </span>

      <button
        type="button"
        onclick={() => store.showPodPrefix = !store.showPodPrefix}
        class="px-2 py-0.5 text-xs border rounded transition-colors
          {store.showPodPrefix
            ? 'border-accent text-accent bg-accent/10'
            : 'border-border text-muted hover:bg-surface-hover'}"
      >
        Pod prefix
      </button>

      <div class="flex items-center gap-1 ml-auto">
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

    <!-- Log area -->
    <div bind:this={scrollEl} class="flex-1 overflow-auto font-mono bg-bg" onscroll={onScroll}>
      <div style:height="{$virtualizerStore.getTotalSize()}px" style:position="relative">
        {#each $virtualizerStore.getVirtualItems() as row (row.index)}
          {@const line = store.lines[row.index]}
          {#if line}
            <div
              style:position="absolute"
              style:top="0"
              style:left="0"
              style:width="100%"
              style:transform="translateY({row.start}px)"
              class="px-3 leading-5 whitespace-pre log-row"
            >
              {#if store.showPodPrefix}
                <span style="color: {line.color}" class="mr-1 select-none">[{line.pod}]</span>
              {/if}
              {line.text}
            </div>
          {/if}
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  .log-row {
    font-size: var(--log-font-size, 13px);
  }
</style>
