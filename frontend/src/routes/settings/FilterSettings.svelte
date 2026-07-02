<script lang="ts">
  import {onMount} from "svelte";
  import {
    GetConfig,
    GetClusterPrefs,
    SetClusterSavedFilters,
    SetSavedFilters,
  } from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
  import type {SavedFilter} from "$api/github.com/Vilsol/klados/internal/config/models.js";
  import {clusterStore} from "$lib/stores/cluster.svelte";

  type FilterScope = "global" | "cluster";

  let filtersByGVR = $state<Record<string, SavedFilter[]>>({});
  let clusterFiltersByGVR = $state<Record<string, SavedFilter[]>>({});
  let newGVR = $state<string>("");
  let newClusterGVR = $state<string>("");

  let editingGVR = $state<string | null>(null);
  let editingIndex = $state<number>(-1);
  let editingScope = $state<FilterScope>("global");
  let editName = $state<string>("");
  let editLabels = $state<string>("");
  let editAnnotations = $state<string>("");
  let editSearch = $state<string>("");
  let showModal = $state<boolean>(false);

  let activeContext = $derived(clusterStore.activeContext);

  onMount(() => {
    loadGlobalFilters();
    loadClusterFilters();
  });

  async function loadGlobalFilters() {
    const config = await GetConfig();
    if (config?.savedFilters) {
      filtersByGVR = config.savedFilters as Record<string, SavedFilter[]>;
    }
  }

  async function loadClusterFilters() {
    if (!activeContext) {
      return;
    }
    const prefs = await GetClusterPrefs(activeContext);
    if (prefs?.savedFilters) {
      clusterFiltersByGVR = prefs.savedFilters as Record<string, SavedFilter[]>;
    } else {
      clusterFiltersByGVR = {};
    }
  }

  function parseKV(str: string): Record<string, string> | undefined {
    if (!str.trim()) {
      return;
    }
    const result: Record<string, string> = {};
    for (const pair of str.split(",")) {
      const [key, ...rest] = pair.split("=");
      if (key?.trim() && rest.length > 0) {
        result[key.trim()] = rest.join("=").trim();
      }
    }
    return Object.keys(result).length > 0 ? result : undefined;
  }

  function formatKV(obj?: Record<string, string | undefined>): string {
    if (!obj) {
      return "";
    }
    return Object.entries(obj)
      .filter((entry): entry is [string, string] => entry[1] !== undefined)
      .map(([k, v]) => `${k}=${v}`)
      .join(", ");
  }

  function openAdd(gvr: string, scope: FilterScope = "global") {
    editingGVR = gvr;
    editingIndex = -1;
    editingScope = scope;
    editName = "";
    editLabels = "";
    editAnnotations = "";
    editSearch = "";
    showModal = true;
  }

  function openEdit(gvr: string, index: number, scope: FilterScope = "global") {
    const source = scope === "global" ? filtersByGVR : clusterFiltersByGVR;
    const filter = source[gvr]?.[index];
    if (!filter) {
      return;
    }
    editingGVR = gvr;
    editingIndex = index;
    editingScope = scope;
    editName = filter.name;
    editLabels = formatKV(filter.labels);
    editAnnotations = formatKV(filter.annotations);
    editSearch = filter.search ?? "";
    showModal = true;
  }

  function closeModal() {
    showModal = false;
    editingGVR = null;
  }

  async function saveFilter() {
    if (!(editingGVR && editName.trim())) {
      return;
    }
    const filter: SavedFilter = {
      name: editName.trim(),
      labels: parseKV(editLabels),
      annotations: parseKV(editAnnotations),
      search: editSearch.trim() || undefined,
    };

    const gvr = editingGVR;
    if (editingScope === "cluster" && activeContext) {
      const existing = [...(clusterFiltersByGVR[gvr] ?? [])];
      if (editingIndex >= 0) {
        existing[editingIndex] = filter;
      } else {
        existing.push(filter);
      }
      clusterFiltersByGVR = {...clusterFiltersByGVR, [gvr]: existing};
      await SetClusterSavedFilters(activeContext, gvr, existing);
    } else {
      const existing = [...(filtersByGVR[gvr] ?? [])];
      if (editingIndex >= 0) {
        existing[editingIndex] = filter;
      } else {
        existing.push(filter);
      }
      filtersByGVR = {...filtersByGVR, [gvr]: existing};
      await SetSavedFilters(gvr, existing);
    }
    closeModal();
  }

  async function deleteFilter(gvr: string, index: number, scope: FilterScope = "global") {
    if (scope === "cluster" && activeContext) {
      const existing = [...(clusterFiltersByGVR[gvr] ?? [])];
      existing.splice(index, 1);
      if (existing.length === 0) {
        const {[gvr]: _, ...rest} = clusterFiltersByGVR;
        clusterFiltersByGVR = rest;
      } else {
        clusterFiltersByGVR = {...clusterFiltersByGVR, [gvr]: existing};
      }
      await SetClusterSavedFilters(activeContext, gvr, existing);
    } else {
      const existing = [...(filtersByGVR[gvr] ?? [])];
      existing.splice(index, 1);
      if (existing.length === 0) {
        const {[gvr]: _, ...rest} = filtersByGVR;
        filtersByGVR = rest;
      } else {
        filtersByGVR = {...filtersByGVR, [gvr]: existing};
      }
      await SetSavedFilters(gvr, existing);
    }
  }

  function addGVR() {
    const gvr = newGVR.trim();
    if (gvr && !(gvr in filtersByGVR)) {
      filtersByGVR = {...filtersByGVR, [gvr]: []};
      newGVR = "";
    }
  }

  function addClusterGVR() {
    const gvr = newClusterGVR.trim();
    if (gvr && !(gvr in clusterFiltersByGVR)) {
      clusterFiltersByGVR = {...clusterFiltersByGVR, [gvr]: []};
      newClusterGVR = "";
    }
  }

  let gvrKeys = $derived(Object.keys(filtersByGVR).sort());
  let clusterGvrKeys = $derived(Object.keys(clusterFiltersByGVR).sort());
</script>

{#if showModal}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onclick={closeModal}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="bg-bg border border-border rounded-lg p-6 w-96 space-y-4" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-base font-medium text-fg">{editingIndex >= 0 ? 'Edit' : 'Add'} Filter</h3>

      <div>
        <label class="block text-sm font-medium text-fg mb-1"
          >Name
          <input type="text" bind:value={editName} class="w-full px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm">
        </label>
      </div>

      <div>
        <label class="block text-sm font-medium text-fg mb-1"
          >Labels
          <input
            type="text"
            bind:value={editLabels}
            placeholder="key=value, key2=value2"
            class="w-full px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm"
          >
        </label>
        <p class="text-xs text-muted-foreground mt-1">Comma-separated key=value pairs</p>
      </div>

      <div>
        <label class="block text-sm font-medium text-fg mb-1"
          >Annotations
          <input
            type="text"
            bind:value={editAnnotations}
            placeholder="key=value, key2=value2"
            class="w-full px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm"
          >
        </label>
      </div>

      <div>
        <label class="block text-sm font-medium text-fg mb-1"
          >Search Text
          <input type="text" bind:value={editSearch} class="w-full px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm">
        </label>
      </div>

      <div class="flex justify-end gap-2 pt-2">
        <button type="button" class="px-3 py-1.5 rounded border border-border text-fg text-sm hover:bg-surface-hover" onclick={closeModal}>
          Cancel
        </button>
        <button type="button" class="px-3 py-1.5 rounded bg-accent text-accent-foreground text-sm hover:opacity-90" onclick={saveFilter}>
          Save
        </button>
      </div>
    </div>
  </div>
{/if}

<div class="max-w-3xl space-y-6">
  <h2 class="text-base font-medium text-fg">Global Saved Filters</h2>
  <p class="text-sm text-muted">These filters apply to all clusters.</p>

  {#each gvrKeys as gvr}
    <div class="border border-border rounded">
      <div class="flex items-center justify-between px-4 py-2 bg-surface border-b border-border">
        <span class="text-sm font-mono text-fg">{gvr}</span>
        <button type="button" class="text-sm text-accent hover:underline" onclick={() => openAdd(gvr, 'global')}>+ Add filter</button>
      </div>
      {#if (filtersByGVR[gvr] ?? []).length === 0}
        <div class="px-4 py-3 text-sm text-muted">No filters for this resource type.</div>
      {:else}
        {#each filtersByGVR[gvr] ?? [] as filter, i}
          <div class="flex items-center justify-between px-4 py-2 border-b border-border last:border-0">
            <div>
              <span class="text-sm text-fg font-medium">{filter.name}</span>
              {#if filter.search}
                <span class="text-xs text-muted ml-2">search: {filter.search}</span>
              {/if}
            </div>
            <div class="flex gap-2">
              <button type="button" class="text-xs text-muted hover:text-fg" onclick={() => openEdit(gvr, i, 'global')}>Edit</button>
              <button type="button" class="text-xs text-destructive hover:underline" onclick={() => deleteFilter(gvr, i, 'global')}>
                Delete
              </button>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  {/each}

  {#if gvrKeys.length === 0}
    <p class="text-sm text-muted">No global saved filters. Add a resource type to get started.</p>
  {/if}

  <div>
    <h3 class="text-sm font-medium text-fg mb-2">Add resource type</h3>
    <div class="flex gap-2">
      <input
        type="text"
        bind:value={newGVR}
        placeholder="e.g. apps.v1.deployments"
        class="flex-1 px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm"
        onkeydown={(e) => e.key === 'Enter' && addGVR()}
      >
      <button type="button" class="px-3 py-1.5 rounded bg-accent text-accent-foreground text-sm hover:opacity-90" onclick={addGVR}>
        Add
      </button>
    </div>
  </div>

  <div class="border-t border-border pt-6">
    <h2 class="text-base font-medium text-fg">Cluster-Local Saved Filters</h2>
    {#if activeContext}
      <p class="text-sm text-muted">Filters scoped to cluster <span class="font-mono">{activeContext}</span>.</p>

      {#each clusterGvrKeys as gvr}
        <div class="border border-border rounded mt-4">
          <div class="flex items-center justify-between px-4 py-2 bg-surface border-b border-border">
            <span class="text-sm font-mono text-fg">{gvr}</span>
            <button type="button" class="text-sm text-accent hover:underline" onclick={() => openAdd(gvr, 'cluster')}>+ Add filter</button>
          </div>
          {#if (clusterFiltersByGVR[gvr] ?? []).length === 0}
            <div class="px-4 py-3 text-sm text-muted">No filters for this resource type.</div>
          {:else}
            {#each clusterFiltersByGVR[gvr] ?? [] as filter, i}
              <div class="flex items-center justify-between px-4 py-2 border-b border-border last:border-0">
                <div>
                  <span class="text-sm text-fg font-medium">{filter.name}</span>
                  {#if filter.search}
                    <span class="text-xs text-muted ml-2">search: {filter.search}</span>
                  {/if}
                </div>
                <div class="flex gap-2">
                  <button type="button" class="text-xs text-muted hover:text-fg" onclick={() => openEdit(gvr, i, 'cluster')}>Edit</button>
                  <button type="button" class="text-xs text-destructive hover:underline" onclick={() => deleteFilter(gvr, i, 'cluster')}>
                    Delete
                  </button>
                </div>
              </div>
            {/each}
          {/if}
        </div>
      {/each}

      {#if clusterGvrKeys.length === 0}
        <p class="text-sm text-muted mt-4">No cluster-local saved filters for this cluster.</p>
      {/if}

      <div class="mt-4">
        <h3 class="text-sm font-medium text-fg mb-2">Add resource type</h3>
        <div class="flex gap-2">
          <input
            type="text"
            bind:value={newClusterGVR}
            placeholder="e.g. apps.v1.deployments"
            class="flex-1 px-3 py-1.5 rounded border border-border bg-surface text-fg text-sm"
            onkeydown={(e) => e.key === 'Enter' && addClusterGVR()}
          >
          <button
            type="button"
            class="px-3 py-1.5 rounded bg-accent text-accent-foreground text-sm hover:opacity-90"
            onclick={addClusterGVR}
          >
            Add
          </button>
        </div>
      </div>
    {:else}
      <p class="text-sm text-muted mt-2">Connect to a cluster to manage cluster-local filters.</p>
    {/if}
  </div>
</div>
