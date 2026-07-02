<script lang="ts">
  import {Dialog} from "bits-ui";
  import {selectionStore} from "$lib/stores/selection.svelte";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {Check, X, Loader2} from "lucide-svelte";
  import {ScaleResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";

  let {
    open = $bindable(false),
    contextName,
  }: {
    open: boolean;
    contextName: string;
  } = $props();

  type ItemStatus = "pending" | "scaling" | "success" | "error";
  type Mode = "set" | "increase" | "decrease";

  let mode = $state<Mode>("set");
  let value = $state(1);
  let statuses = $state<Map<string, {status: ItemStatus; error?: string}>>(new Map());
  let running = $state(false);

  const selectedItems = $derived(selectionStore.items());
  const gvr = $derived(selectionStore.selectedGVR);

  $effect(() => {
    if (open) {
      mode = "set";
      value = 1;
      statuses = new Map();
      running = false;
    }
  });

  function itemKey(obj: import("$lib/types").KubernetesResource): string {
    const ns = obj.metadata?.namespace ?? "";
    const name = obj.metadata?.name ?? "";
    return ns ? `${ns}/${name}` : name;
  }

  function currentReplicas(item: import("$lib/types").KubernetesResource): number {
    return item.spec?.replicas ?? 0;
  }

  function targetReplicas(item: import("$lib/types").KubernetesResource): number {
    const current = currentReplicas(item);
    switch (mode) {
      case "set":
        return Math.max(0, value);
      case "increase":
        return current + value;
      case "decrease":
        return Math.max(0, current - value);
    }
  }

  async function run() {
    running = true;
    const items = [...selectedItems];
    statuses = new Map(items.map((item) => [itemKey(item), {status: "pending" as ItemStatus}]));

    const succeeded: string[] = [];
    let failCount = 0;

    for (const item of items) {
      const key = itemKey(item);
      const ns = item.metadata?.namespace ?? "";
      const name = item.metadata?.name ?? "";
      const target = targetReplicas(item);

      statuses = new Map(statuses).set(key, {status: "scaling"});

      try {
        // biome-ignore lint/performance/noAwaitInLoops: sequential for per-item status updates
        await ScaleResource(contextName, gvr, ns, name, target);
        statuses = new Map(statuses).set(key, {status: "success"});
        succeeded.push(key);
      } catch (e: unknown) {
        statuses = new Map(statuses).set(key, {status: "error", error: (e as {message?: string})?.message ?? String(e)});
        failCount++;
      }
    }

    selectionStore.deselectKeys(succeeded);
    running = false;

    if (failCount === 0) {
      notificationStore.push(`Scaled ${succeeded.length}/${items.length} resources`, "success");
      open = false;
    } else {
      notificationStore.push(`Scaled ${succeeded.length}/${items.length} — ${failCount} failed`, "error");
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl p-6 w-[480px] max-w-[90vw] max-h-[80vh] flex flex-col"
    >
      <Dialog.Title class="text-base font-semibold mb-4">Scale {selectedItems.length} resources</Dialog.Title>

      <div class="flex gap-0 mb-4 rounded border border-border overflow-hidden">
        {#each (['set', 'increase', 'decrease'] as Mode[]) as m}
          <button
            type="button"
            onclick={() => (mode = m)}
            class="flex-1 px-3 py-1.5 text-sm border-r border-border last:border-r-0 transition-colors capitalize {mode === m ? 'bg-accent text-accent-fg border-accent' : 'hover:bg-surface-hover'}"
          >
            {#if m === 'set'}
              Set to
            {:else if m === 'increase'}
              Increase by
            {:else}
              Decrease by
            {/if}
          </button>
        {/each}
      </div>

      <div class="mb-4">
        <input
          type="number"
          min="0"
          bind:value
          class="w-full px-3 py-1.5 text-sm border border-border rounded bg-surface focus:outline-none focus:ring-1 focus:ring-accent"
        >
      </div>

      <div class="flex-1 overflow-auto mb-4 border border-border rounded">
        <div class="grid grid-cols-[1fr_80px_20px_80px] px-3 py-1 text-xs text-muted border-b border-border">
          <span>Name</span>
          <span class="text-right">Current</span>
          <span></span>
          <span class="text-right">Target</span>
        </div>
        {#each selectedItems as item}
          {@const key = itemKey(item)}
          {@const st = statuses.get(key)}
          {@const current = currentReplicas(item)}
          {@const target = targetReplicas(item)}
          <div class="grid grid-cols-[1fr_80px_20px_80px] items-center px-3 py-1.5 text-sm border-b border-border last:border-b-0">
            <div class="flex items-center gap-2 min-w-0">
              <span class="w-4 flex-shrink-0">
                {#if st?.status === 'success'}
                  <Check size={14} class="text-accent" />
                {:else if st?.status === 'error'}
                  <X size={14} class="text-destructive" />
                {:else if st?.status === 'scaling'}
                  <Loader2 size={14} class="animate-spin text-muted" />
                {/if}
              </span>
              <span class="truncate">{item.metadata?.namespace ? `${item.metadata.namespace}/` : ''}{item.metadata?.name}</span>
            </div>
            <span class="text-right text-muted">{current}</span>
            <span class="text-center text-muted">→</span>
            <span class="text-right {target !== current ? 'text-accent' : ''}">{target}</span>
          </div>
        {/each}
      </div>

      <div class="flex justify-end gap-2">
        <Dialog.Close class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors" disabled={running}
          >Cancel</Dialog.Close
        >
        <button
          type="button"
          onclick={run}
          disabled={running}
          class="px-3 py-1.5 text-sm rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {running ? 'Scaling…' : 'Scale'}
        </button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
