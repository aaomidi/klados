<script lang="ts">
  import {Events} from "@wailsio/runtime";
  import {IsActive, CancelDrain} from "$api/github.com/Vilsol/klados/internal/services/drainservice.js";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import type {KubernetesResource} from "$lib/types";

  let {
    ctxName,
    name,
  }: {
    ctxName: string;
    name: string;
    namespace: string;
    obj: Record<string, KubernetesResource>;
  } = $props();

  let lines = $state<string[]>([]);
  let terminalState = $state<"active" | "complete" | "error" | "cancelled">("active");
  let isActive = $state(false);
  let logEl = $state<HTMLPreElement | null>(null);

  $effect(() => {
    const eventName = `drain:${ctxName}:${name}`;

    IsActive(ctxName, name).then((active: boolean) => {
      isActive = active;
      if (active) {
        terminalState = "active";
      }
    });

    const unsub = Events.On(eventName, (wailsEvent: KubernetesResource) => {
      const msg = wailsEvent.data;
      if (!msg) {
        return;
      }
      switch (msg.type) {
        case "log":
          lines = [...lines, msg.message];
          break;
        case "error":
          lines = [...lines, `ERROR: ${msg.message}`];
          break;
        case "complete":
          isActive = false;
          if (terminalState === "active") {
            terminalState = "complete";
          }
          break;
        case "cancelled":
          isActive = false;
          terminalState = "cancelled";
          break;
      }
    });

    const unsubUpdated = Events.On(`drain:${ctxName}:updated`, () => {
      IsActive(ctxName, name).then((active: boolean) => {
        isActive = active;
      });
    });

    return () => {
      unsub();
      unsubUpdated();
    };
  });

  $effect(() => {
    if (lines.length > 0 && logEl) {
      logEl.scrollTop = logEl.scrollHeight;
    }
  });

  async function cancelDrain() {
    try {
      await CancelDrain(ctxName, name);
      terminalState = "cancelled";
    } catch (e: unknown) {
      notificationStore.push(e instanceof Error ? e.message : "Cancel failed", "error");
    }
  }
</script>

<div class="flex flex-col h-full overflow-hidden p-4 gap-3">
  <div class="flex items-center justify-between">
    <h3 class="text-sm font-medium text-fg">Drain Log</h3>
    {#if isActive}
      <button
        type="button"
        onclick={cancelDrain}
        class="text-xs px-2.5 py-1 rounded border border-destructive text-destructive hover:bg-destructive/10 transition-colors"
      >
        Cancel Drain
      </button>
    {:else if terminalState !== 'active'}
      <span
        class="text-xs px-2 py-0.5 rounded font-medium {terminalState === 'complete' ? 'bg-green-500/20 text-green-400' : terminalState === 'error' ? 'bg-red-500/20 text-red-400' : 'bg-muted/30 text-muted'}"
      >
        {#if terminalState === 'complete'}
          Completed
        {:else if terminalState === 'error'}
          Failed
        {:else}
          Cancelled
        {/if}
      </span>
    {/if}
  </div>

  {#if lines.length === 0 && !isActive}
    <p class="text-xs text-muted">No drain activity. Use the Drain action to start draining this node.</p>
  {:else}
    <pre
      bind:this={logEl}
      class="flex-1 overflow-auto text-xs font-mono bg-bg border border-border rounded p-3 whitespace-pre-wrap"
    >{lines.join('\n')}</pre>
  {/if}
</div>
