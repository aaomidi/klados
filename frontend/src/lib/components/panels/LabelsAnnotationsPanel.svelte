<script lang="ts">
  import {UpdateResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {SectionHeader, KeyValueBadge, EmptyState, KeyValuePairEditor} from "@klados/ui";
  import type {KubernetesResource} from "$lib/types";

  let {
    obj,
    onupdate,
    ctxName,
    gvr,
    namespace,
    name,
  }: {
    obj: Record<string, KubernetesResource>;
    onupdate?: (updated: Record<string, KubernetesResource>) => void;
    ctxName: string;
    gvr: string;
    namespace: string;
    name: string;
  } = $props();

  let editing = $state(false);
  let saving = $state(false);

  let editLabels = $state<[string, string][]>([]);
  let editAnnotations = $state<[string, string][]>([]);

  function startEdit() {
    editLabels = Object.entries(obj.metadata?.labels ?? {}).map(([k, v]) => [k, String(v)]);
    editAnnotations = Object.entries(obj.metadata?.annotations ?? {}).map(([k, v]) => [k, String(v)]);
    editing = true;
  }

  function cancelEdit() {
    editing = false;
  }

  async function save() {
    saving = true;
    try {
      const updated = JSON.parse(JSON.stringify(obj));
      updated.metadata.labels = Object.fromEntries(editLabels.filter(([k]) => k.trim()));
      updated.metadata.annotations = Object.fromEntries(editAnnotations.filter(([k]) => k.trim()));
      const result = await UpdateResource(ctxName, gvr, namespace, updated);
      if (result) {
        onupdate?.(result);
      }
      editing = false;
      notificationStore.push("Labels and annotations saved.", "success");
    } catch (e: unknown) {
      notificationStore.push(e instanceof Error ? e.message : "Save failed", "error");
    } finally {
      saving = false;
    }
  }

  const labels = $derived(Object.entries(obj.metadata?.labels ?? {}));
  const annotations = $derived(Object.entries(obj.metadata?.annotations ?? {}));
</script>

<div class="p-4 flex flex-col gap-6 overflow-auto">
  <div class="flex items-center justify-between">
    <SectionHeader class="">Labels & Annotations</SectionHeader>
    {#if !editing}
      <button
        type="button"
        onclick={startEdit}
        class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
      >
        Edit
      </button>
    {:else}
      <div class="flex gap-2">
        <button
          type="button"
          onclick={cancelEdit}
          class="text-xs px-2.5 py-1 rounded border border-border hover:bg-surface-hover transition-colors"
        >
          Cancel
        </button>
        <button
          type="button"
          onclick={save}
          disabled={saving}
          class="text-xs px-2.5 py-1 rounded bg-accent text-accent-fg hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
      </div>
    {/if}
  </div>

  {#if !editing}
    <section>
      <h3 class="text-xs font-medium mb-2">Labels</h3>
      <KeyValueBadge entries={labels} />
    </section>

    <section>
      <h3 class="text-xs font-medium mb-2">Annotations</h3>
      {#if annotations.length === 0}
        <EmptyState />
      {:else}
        <div class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
          {#each annotations.sort(([a], [b]) => a.localeCompare(b)) as [ k, v ]}
            <span class="text-xs font-mono text-muted">{k}</span>
            <span class="text-xs font-mono break-all">{v}</span>
          {/each}
        </div>
      {/if}
    </section>
  {:else}
    <section>
      <h3 class="text-xs font-medium mb-2">Labels</h3>
      <KeyValuePairEditor bind:pairs={editLabels} addLabel="+ Add label" />
    </section>

    <section>
      <h3 class="text-xs font-medium mb-2">Annotations</h3>
      <KeyValuePairEditor bind:pairs={editAnnotations} addLabel="+ Add annotation" />
    </section>
  {/if}
</div>
