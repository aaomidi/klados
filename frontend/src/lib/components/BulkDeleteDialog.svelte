<script lang="ts">
  import {Dialog} from "bits-ui";
  import {selectionStore} from "$lib/stores/selection.svelte";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {Check, X, Loader2} from "lucide-svelte";
  import {DeleteResource, ForceDeleteResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {shortcutActions} from "$lib/stores/shortcutActions.svelte";

  let {
    open = $bindable(false),
    contextName,
  }: {
    open: boolean;
    contextName: string;
  } = $props();

  type ItemStatus = "pending" | "deleting" | "success" | "error";
  let statuses = $state<Map<string, {status: ItemStatus; error?: string}>>(new Map());
  let running = $state(false);
  let force = $state(false);

  $effect(() => {
    if (!open) {
      force = false;
    }
  });

  const selectedItems = $derived(selectionStore.items());
  const gvr = $derived(selectionStore.selectedGVR);

  function itemKey(obj: import("$lib/types").KubernetesResource): string {
    const ns = obj.metadata?.namespace ?? "";
    const name = obj.metadata?.name ?? "";
    return ns ? `${ns}/${name}` : name;
  }

  $effect(() => {
    shortcutActions.confirmDialog;
    if (shortcutActions.confirmDialog > 0 && open && !running) {
      run();
    }
  });

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

      statuses = new Map(statuses).set(key, {status: "deleting"});

      try {
        const fn = force ? ForceDeleteResource : DeleteResource;
        // biome-ignore lint/performance/noAwaitInLoops: sequential for per-item status updates
        await fn(contextName, gvr, ns, name);
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
      notificationStore.push(`Deleted ${succeeded.length}/${items.length} resources`, "success");
      open = false;
    } else {
      notificationStore.push(`Deleted ${succeeded.length}/${items.length} — ${failCount} failed`, "error");
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl p-6 w-[480px] max-w-[90vw] max-h-[70vh] flex flex-col"
    >
      <Dialog.Title class="text-base font-semibold mb-2">Delete {selectedItems.length} resources</Dialog.Title>
      <Dialog.Description class="text-sm text-muted mb-4">This action cannot be undone.</Dialog.Description>

      <div class="flex-1 overflow-auto mb-4 border border-border rounded">
        {#each selectedItems as item}
          {@const key = itemKey(item)}
          {@const st = statuses.get(key)}
          <div class="flex items-center gap-2 px-3 py-1.5 text-sm border-b border-border last:border-b-0">
            <span class="w-4 flex-shrink-0">
              {#if st?.status === 'success'}
                <Check size={14} class="text-accent" />
              {:else if st?.status === 'error'}
                <X size={14} class="text-destructive" />
              {:else if st?.status === 'deleting'}
                <Loader2 size={14} class="animate-spin text-muted" />
              {/if}
            </span>
            <span class="truncate flex-1">{item.metadata?.namespace ? `${item.metadata.namespace}/` : ''}{item.metadata?.name}</span>
            {#if st?.error}
              <span class="text-xs text-destructive truncate max-w-48" title={st.error}>{st.error}</span>
            {/if}
          </div>
        {/each}
      </div>

      <label class="flex items-start gap-2 mt-3 text-sm cursor-pointer">
        <input type="checkbox" bind:checked={force} class="mt-0.5" />
        <span>
          <span class="font-medium">Force delete</span>
          <span class="text-muted">(skip graceful shutdown)</span>
        </span>
      </label>
      {#if force}
        <p class="mt-2 text-xs text-destructive">
          Bypasses graceful termination. May leave dangling resources (etcd entries, finalizers). Use only for stuck objects.
        </p>
      {/if}

      <div class="flex justify-end gap-2 mt-3">
        <Dialog.Close class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors" disabled={running}
          >Cancel</Dialog.Close
        >
        <button
          type="button"
          onclick={run}
          disabled={running}
          class="px-3 py-1.5 text-sm rounded bg-destructive text-destructive-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {#if running}
            Deleting…
          {:else}
            {force ? 'Force Delete' : 'Delete'} {selectedItems.length} {selectedItems.length === 1 ? 'item' : 'items'}
          {/if}
        </button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
