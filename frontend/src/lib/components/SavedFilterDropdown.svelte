<script lang="ts">
  import {preferencesStore, type SavedFilter} from "$lib/stores/preferences.svelte";
  import {savedFilterToQuery, queryToSavedFilter} from "$lib/search/serialize";
  import {Bookmark, X} from "lucide-svelte";
  import {
    SetClusterSavedFilters,
    SetSavedFilters,
    GetSavedFilters,
    GetClusterPrefs,
  } from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
  import {SavedFilter as BindingSavedFilter} from "$api/github.com/Vilsol/klados/internal/config/models.js";

  function toBindingFilters(filters: SavedFilter[]): BindingSavedFilter[] {
    return filters.map((f) => new BindingSavedFilter(f));
  }

  let {
    gvr,
    contextName,
    currentQuery = "",
    onapply,
  }: {
    gvr: string;
    contextName: string;
    currentQuery: string;
    onapply?: (query: string) => void;
  } = $props();

  let dropdownOpen = $state(false);
  let showSaveForm = $state(false);
  let saveName = $state("");
  let saveScope = $state<"cluster" | "global">("cluster");
  let confirmOverwrite = $state(false);

  let savedFilters = $derived(preferencesStore.getSavedFilters(gvr));

  function toggleDropdown() {
    dropdownOpen = !dropdownOpen;
    if (!dropdownOpen) {
      showSaveForm = false;
    }
  }

  function applyFilter(filter: SavedFilter) {
    onapply?.(savedFilterToQuery(filter));
    dropdownOpen = false;
  }

  function openSaveForm() {
    showSaveForm = true;
    saveName = "";
    saveScope = "cluster";
  }

  function isDuplicate(): boolean {
    return savedFilters.some((f) => f.name === saveName.trim());
  }

  async function saveFilter(force = false) {
    if (!saveName.trim()) {
      return;
    }

    if (!force && isDuplicate()) {
      confirmOverwrite = true;
      return;
    }

    confirmOverwrite = false;
    const filterData = queryToSavedFilter(currentQuery);
    const newFilter: SavedFilter = {name: saveName.trim(), ...filterData};

    const existing = [...savedFilters];
    const dupeIdx = existing.findIndex((f) => f.name === newFilter.name);
    if (dupeIdx >= 0) {
      existing[dupeIdx] = newFilter;
    } else {
      existing.push(newFilter);
    }

    if (saveScope === "cluster") {
      await SetClusterSavedFilters(contextName, gvr, toBindingFilters(existing));
    } else {
      await SetSavedFilters(gvr, toBindingFilters(existing));
    }

    showSaveForm = false;
    dropdownOpen = false;
  }

  async function deleteFilter(filter: SavedFilter) {
    const name = filter.name;
    // Remove from both global and cluster scopes (we don't know which one it's from)
    const globalFilters = await GetSavedFilters(gvr);
    const globalUpdated = (globalFilters ?? []).filter((f: {name?: string}) => f.name !== name);
    if (globalUpdated.length !== (globalFilters ?? []).length) {
      await SetSavedFilters(gvr, globalUpdated);
    }

    const clusterPrefs = await GetClusterPrefs(contextName);
    const clusterFilters = (clusterPrefs as {savedFilters?: Record<string, {name?: string}[]>})?.savedFilters?.[gvr] ?? [];
    const clusterUpdated = clusterFilters.filter((f: {name?: string}) => f.name !== name);
    if (clusterUpdated.length !== clusterFilters.length) {
      await SetClusterSavedFilters(contextName, gvr, toBindingFilters(clusterUpdated.filter((f): f is SavedFilter => typeof f.name === "string")));
    }
  }

  function filterPreview(filter: SavedFilter): string {
    return savedFilterToQuery(filter) || "(empty)";
  }

  function handleClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement;
    if (!target.closest(".saved-filter-dropdown")) {
      dropdownOpen = false;
    }
  }
</script>

<svelte:window onclick={handleClickOutside} />

<div class="relative saved-filter-dropdown">
  <button
    type="button"
    class="p-1.5 rounded text-muted hover:text-fg hover:bg-surface-hover"
    title="Saved filters"
    onclick={toggleDropdown}
  >
    <Bookmark size={16} />
  </button>

  {#if dropdownOpen}
    <div class="absolute right-0 top-full mt-1 z-50 bg-surface border border-border rounded shadow-lg py-1 min-w-64">
      {#if !showSaveForm}
        <div class="px-3 py-1.5 text-xs font-medium text-muted uppercase tracking-wide">Saved Filters</div>

        {#if savedFilters.length === 0}
          <div class="px-3 py-2 text-sm text-muted">No saved filters for this resource</div>
        {/if}

        {#each savedFilters as filter}
          <div class="flex items-center hover:bg-surface-hover group">
            <button type="button" class="flex-1 text-left px-3 py-1.5" onclick={() => applyFilter(filter)}>
              <div class="text-sm text-fg font-medium">{filter.name}</div>
              <div class="text-xs text-muted font-mono truncate">{filterPreview(filter)}</div>
            </button>
            <button
              type="button"
              class="p-1 mr-2 rounded text-muted hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
              title="Delete filter"
              onclick={(e) => { e.stopPropagation(); deleteFilter(filter) }}
            >
              <X size={14} />
            </button>
          </div>
        {/each}

        <div class="border-t border-border mt-1 pt-1">
          <button
            type="button"
            class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover disabled:opacity-50 disabled:cursor-not-allowed text-accent"
            disabled={!currentQuery.trim()}
            onclick={openSaveForm}
          >
            + Save current filter
          </button>
        </div>
      {:else}
        <div class="px-3 py-2">
          <div class="text-xs font-medium text-muted uppercase tracking-wide mb-2">Save Filter</div>

          <input
            bind:value={saveName}
            class="w-full px-2 py-1 text-sm bg-surface border border-border rounded text-fg placeholder:text-muted mb-2"
            placeholder="Filter name"
            onkeydown={(e) => e.key === 'Enter' && saveFilter()}
          >

          <div class="flex gap-3 mb-3 text-sm">
            <label class="flex items-center gap-1.5 text-fg">
              <input type="radio" bind:group={saveScope} value="cluster" class="accent-accent">
              This cluster
            </label>
            <label class="flex items-center gap-1.5 text-fg">
              <input type="radio" bind:group={saveScope} value="global" class="accent-accent">
              Global
            </label>
          </div>

          {#if confirmOverwrite}
            <div class="text-xs text-destructive mb-2">A filter named "{saveName.trim()}" already exists. Overwrite it?</div>
            <div class="flex justify-end gap-2">
              <button type="button" class="px-2 py-1 text-sm rounded text-muted hover:text-fg" onclick={() => { confirmOverwrite = false }}>
                Cancel
              </button>
              <button
                type="button"
                class="px-2 py-1 text-sm rounded bg-destructive text-white hover:opacity-90"
                onclick={() => saveFilter(true)}
              >
                Overwrite
              </button>
            </div>
          {:else}
            <div class="flex justify-end gap-2">
              <button type="button" class="px-2 py-1 text-sm rounded text-muted hover:text-fg" onclick={() => (showSaveForm = false)}>
                Cancel
              </button>
              <button
                type="button"
                class="px-2 py-1 text-sm rounded bg-accent text-white hover:opacity-90 disabled:opacity-50"
                disabled={!saveName.trim()}
                onclick={() => saveFilter()}
              >
                Save
              </button>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>
