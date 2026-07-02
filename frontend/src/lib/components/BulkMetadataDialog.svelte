<script lang="ts">
  import {Dialog} from "bits-ui";
  import {KeyValuePairEditor} from "@klados/ui";
  import {selectionStore} from "$lib/stores/selection.svelte";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {Check, X, Loader2} from "lucide-svelte";
  import {UpdateResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";

  let {
    open = $bindable(false),
    mode,
    contextName,
    gvr,
  }: {
    open: boolean;
    mode: "labels" | "annotations";
    contextName: string;
    gvr: string;
  } = $props();

  type ItemStatus = "pending" | "patching" | "success" | "error";
  let statuses = $state<Map<string, {status: ItemStatus; error?: string}>>(new Map());
  let running = $state(false);
  let pairs = $state<[string, string][]>([]);
  let removeKeys = $state<string[]>([]);

  const selectedItems = $derived(selectionStore.items());
  const metadataField = $derived(mode === "labels" ? "labels" : "annotations");
  const title = $derived(mode === "labels" ? "Edit Labels" : "Edit Annotations");

  const commonEntries = $derived.by(() => {
    const items = selectedItems;
    if (items.length === 0) {
      return [];
    }
    const first = items[0].metadata?.[metadataField] ?? {};
    const intersection: [string, string][] = [];
    for (const [k, v] of Object.entries(first)) {
      if (items.every((item) => item.metadata?.[metadataField]?.[k] === v)) {
        intersection.push([k, String(v)]);
      }
    }
    return intersection;
  });

  const allKeys = $derived.by(() => {
    const keys = new Set<string>();
    for (const item of selectedItems) {
      for (const k of Object.keys(item.metadata?.[metadataField] ?? {})) {
        keys.add(k);
      }
    }
    return [...keys].sort();
  });

  const hasChanges = $derived(pairs.length > 0 || removeKeys.length > 0);

  $effect(() => {
    if (open) {
      pairs = commonEntries.map(([k, v]) => [k, v] as [string, string]);
      removeKeys = [];
      statuses = new Map();
      running = false;
    }
  });

  function itemKey(obj: import("$lib/types").KubernetesResource): string {
    const ns = obj.metadata?.namespace ?? "";
    const name = obj.metadata?.name ?? "";
    return ns ? `${ns}/${name}` : name;
  }

  function toggleRemoveKey(key: string) {
    if (removeKeys.includes(key)) {
      removeKeys = removeKeys.filter((k) => k !== key);
    } else {
      removeKeys = [...removeKeys, key];
      pairs = pairs.filter(([k]) => k !== key);
    }
  }

  async function apply() {
    running = true;
    const items = [...selectedItems];
    statuses = new Map(items.map((item) => [itemKey(item), {status: "pending" as ItemStatus}]));

    const pairsSnapshot = [...pairs];
    const removeSnapshot = [...removeKeys];

    const succeeded: string[] = [];
    let failCount = 0;

    for (const item of items) {
      const key = itemKey(item);
      const ns = item.metadata?.namespace ?? "";

      statuses = new Map(statuses).set(key, {status: "patching"});

      try {
        const updated = JSON.parse(JSON.stringify(item));
        if (!updated.metadata) {
          updated.metadata = {};
        }
        if (!updated.metadata[metadataField]) {
          updated.metadata[metadataField] = {};
        }

        for (const k of removeSnapshot) {
          delete updated.metadata[metadataField][k];
        }
        for (const [k, v] of pairsSnapshot) {
          if (k) {
            updated.metadata[metadataField][k] = v;
          }
        }

        // biome-ignore lint/performance/noAwaitInLoops: sequential for per-item status updates
        await UpdateResource(contextName, gvr, ns, updated);
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
      notificationStore.push(`Updated ${mode} on ${succeeded.length}/${items.length} resources`, "success");
      open = false;
    } else {
      notificationStore.push(`Updated ${succeeded.length}/${items.length} — ${failCount} failed`, "error");
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-40" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 bg-surface border border-border rounded-lg shadow-xl p-6 w-[560px] max-w-[90vw] max-h-[80vh] flex flex-col"
    >
      <Dialog.Title class="text-base font-semibold mb-1">{title}</Dialog.Title>
      <Dialog.Description class="text-sm text-muted mb-4">
        Editing {selectedItems.length} resources. Changes apply to all selected items.
      </Dialog.Description>

      <div class="flex-1 overflow-auto flex flex-col gap-4 min-h-0">
        <section>
          <p class="text-xs font-medium text-muted uppercase tracking-wide mb-2">Add / Update</p>
          <KeyValuePairEditor bind:pairs addLabel={mode === 'labels' ? '+ Add label' : '+ Add annotation'} />
        </section>

        {#if allKeys.length > 0}
          <section>
            <p class="text-xs font-medium text-muted uppercase tracking-wide mb-2">Remove Keys</p>
            <div class="flex flex-wrap gap-1.5">
              {#each allKeys as key}
                {@const marked = removeKeys.includes(key)}
                <button
                  type="button"
                  onclick={() => toggleRemoveKey(key)}
                  class="text-xs px-2 py-0.5 rounded border transition-colors {marked
                    ? 'bg-destructive/10 text-destructive border-destructive/30 line-through'
                    : 'border-border hover:bg-surface-hover'}"
                >
                  {key}
                </button>
              {/each}
            </div>
          </section>
        {/if}

        {#if running || statuses.size > 0}
          <section>
            <p class="text-xs font-medium text-muted uppercase tracking-wide mb-2">Progress</p>
            <div class="border border-border rounded overflow-hidden">
              {#each selectedItems as item}
                {@const key = itemKey(item)}
                {@const st = statuses.get(key)}
                <div class="flex items-center gap-2 px-3 py-1.5 text-sm border-b border-border last:border-b-0">
                  <span class="w-4 flex-shrink-0">
                    {#if st?.status === 'success'}
                      <Check size={14} class="text-accent" />
                    {:else if st?.status === 'error'}
                      <X size={14} class="text-destructive" />
                    {:else if st?.status === 'patching'}
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
          </section>
        {/if}
      </div>

      <div class="flex justify-end gap-2 mt-4">
        <Dialog.Close class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors" disabled={running}
          >Cancel</Dialog.Close
        >
        <button
          type="button"
          onclick={apply}
          disabled={running || !hasChanges}
          class="px-3 py-1.5 text-sm rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {running ? 'Applying…' : 'Apply'}
        </button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
