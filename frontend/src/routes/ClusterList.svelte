<script lang="ts">
  import {onMount} from "svelte";
  import {push} from "svelte-spa-router";
  import {Columns3, Unplug, Settings} from "lucide-svelte";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import type {KubeContext} from "$lib/stores/cluster.svelte";
  import DataTable, {type DataTableColumn} from "$lib/components/DataTable.svelte";
  import ColumnMenu from "$lib/components/ColumnMenu.svelte";
  import KubeconfigImportDialog from "$lib/components/KubeconfigImportDialog.svelte";
  import {
    GetColumnPrefs,
    SetColumnPrefs,
    DeleteColumnPrefs,
  } from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
  import {GVRColumnPrefs, ColumnSettings, SortPrefs} from "$api/github.com/Vilsol/klados/internal/config/models.js";

  const PREFS_KEY = "_clusterList";

  const ALL_COLUMNS: (DataTableColumn & {hidden?: boolean})[] = [
    {name: "Name"},
    {name: "Cluster"},
    {name: "User"},
    {name: "Namespace", hidden: true},
    {name: "Version"},
    {name: "Provider"},
    {name: "Status"},
  ];

  let visibleColumns = $state<DataTableColumn[]>([]);
  let allColumns = $state<{col: DataTableColumn; visible: boolean}[]>([]);
  let sortState = $state<{column: string; direction: "asc" | "desc"} | null>(null);
  let columnMenuOpen = $state(false);
  let showImportDialog = $state(false);
  let saveTimer: ReturnType<typeof setTimeout> | null = null;

  function applyPrefs(prefs: GVRColumnPrefs | null) {
    const poolMap = new Map<string, DataTableColumn>(ALL_COLUMNS.map((c) => [c.name, {...c}]));
    if (prefs?.columns) {
      for (const [name, settings] of Object.entries(prefs.columns)) {
        if (settings?.width !== undefined && poolMap.has(name)) {
          const existing = poolMap.get(name)!;
          poolMap.set(name, {...existing, width: settings.width});
        }
      }
    }

    let visibleNames: string[];
    if (prefs?.order && prefs.order.length > 0) {
      visibleNames = prefs.order.filter((name) => poolMap.has(name));
    } else {
      visibleNames = ALL_COLUMNS.filter((c) => !c.hidden).map((c) => c.name);
    }

    const visibleSet = new Set(visibleNames);
    visibleColumns = visibleNames.map((name) => poolMap.get(name)!);
    allColumns = ALL_COLUMNS.map((c) => ({col: poolMap.get(c.name)!, visible: visibleSet.has(c.name)}));
    sortState = prefs?.sort ? {column: prefs.sort.column, direction: prefs.sort.direction as "asc" | "desc"} : null;
  }

  function buildPrefs(): GVRColumnPrefs {
    return new GVRColumnPrefs({
      order: visibleColumns.map((c) => c.name),
      columns: Object.fromEntries(
        allColumns.filter(({col}) => col.width !== undefined).map(({col}) => [col.name, new ColumnSettings({width: col.width})]),
      ),
      sort: sortState ? new SortPrefs({column: sortState.column, direction: sortState.direction}) : null,
    });
  }

  function savePrefs() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = null;
    SetColumnPrefs(PREFS_KEY, buildPrefs());
  }

  function debouncedSavePrefs() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(savePrefs, 300);
  }

  function setColumnVisible(name: string, visible: boolean) {
    if (name === "Name") return;
    const entry = allColumns.find((e) => e.col.name === name);
    if (!entry || entry.visible === visible) return;
    allColumns = allColumns.map((e) => (e.col.name === name ? {...e, visible} : e));
    if (visible) {
      visibleColumns = [...visibleColumns, entry.col];
    } else {
      visibleColumns = visibleColumns.filter((c) => c.name !== name);
    }
    savePrefs();
  }

  function moveColumn(name: string, direction: "up" | "down") {
    const idx = visibleColumns.findIndex((c) => c.name === name);
    if (idx === -1) return;
    if (direction === "up" && idx === 0) return;
    if (direction === "down" && idx === visibleColumns.length - 1) return;
    const next = [...visibleColumns];
    const swapIdx = direction === "up" ? idx - 1 : idx + 1;
    [next[idx], next[swapIdx]] = [next[swapIdx], next[idx]];
    visibleColumns = next;
    savePrefs();
  }

  function resetColumns() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = null;
    DeleteColumnPrefs(PREFS_KEY);
    applyPrefs(null);
  }

  function getCellValue(ctx: KubeContext, colName: string): string {
    const status = clusterStore.connectionStatus[ctx.name] ?? "disconnected";
    switch (colName) {
      case "Name": return ctx.name;
      case "Cluster": return ctx.cluster;
      case "User": return ctx.user;
      case "Namespace": return ctx.namespace;
      case "Version": return ctx.serverVersion;
      case "Provider": return ctx.provider;
      case "Status": return status;
      default: return "";
    }
  }

  function statusBadgeClass(status: string): string {
    switch (status) {
      case "connected": return "bg-accent/20 text-accent border-accent/30";
      case "connecting": return "bg-yellow-500/20 text-yellow-400 border-yellow-500/30";
      case "error": return "bg-destructive/20 text-destructive border-destructive/30";
      default: return "bg-muted/10 text-fg border-border";
    }
  }

  const sorted = $derived.by(() => {
    let result = [...clusterStore.contexts];
    if (sortState) {
      const {column, direction} = sortState;
      result.sort((a, b) => {
        const av = getCellValue(a, column);
        const bv = getCellValue(b, column);
        const cmp = av.localeCompare(bv);
        return direction === "asc" ? cmp : -cmp;
      });
    }
    return result;
  });

  async function handleRowClick(ctx: KubeContext) {
    const status = clusterStore.connectionStatus[ctx.name] ?? "disconnected";
    if (status !== "connected" && status !== "connecting") {
      clusterStore.connect(ctx.name);
    }
    push(`/c/${encodeURIComponent(ctx.name)}`);
  }

  onMount(() => {
    clusterStore.loadContexts();
    GetColumnPrefs(PREFS_KEY).then((prefs) => applyPrefs(prefs));
  });

  $effect(() => {
    if (!columnMenuOpen) return;
    const close = () => { columnMenuOpen = false };
    const timer = setTimeout(() => window.addEventListener("click", close, {once: true}), 0);
    return () => { clearTimeout(timer); window.removeEventListener("click", close) };
  });
</script>

<KubeconfigImportDialog bind:open={showImportDialog} onsuccess={() => clusterStore.loadContexts()} />

<DataTable
  items={sorted}
  {visibleColumns}
  {sortState}
  compact={false}
  loading={false}
  emptyMessage="No clusters found. Import a kubeconfig to get started."
  suffixGridCols={["36px"]}
  onsort={(col, dir) => { sortState = {column: col, direction: dir}; savePrefs() }}
  onresize={(col, width) => {
    visibleColumns = visibleColumns.map((c) => c.name === col ? {...c, width} : c);
    allColumns = allColumns.map((e) => e.col.name === col ? {...e, col: {...e.col, width}} : e);
    debouncedSavePrefs();
  }}
  onrowclick={handleRowClick}
>
  {#snippet toolbar()}
    <h1 class="text-sm font-semibold">Clusters</h1>
    <span class="text-xs text-muted">{clusterStore.contexts.length} clusters</span>
    <div class="flex-1"></div>
    <div class="relative">
      <button
        type="button"
        onclick={() => columnMenuOpen = !columnMenuOpen}
        class="p-1 rounded hover:bg-surface-hover transition-colors"
        title="Manage columns"
        aria-label="Manage columns"
      >
        <Columns3 size={14} />
      </button>
      {#if columnMenuOpen}
        <ColumnMenu
          {visibleColumns} {allColumns} compact={false}
          onToggle={setColumnVisible}
          onMove={moveColumn}
          onReset={resetColumns}
          onCompactChange={() => {}}
        />
      {/if}
    </div>
    <button
      type="button"
      onclick={() => showImportDialog = true}
      class="px-3 py-1.5 text-sm rounded border border-border hover:bg-surface-hover transition-colors"
    >
      Import Kubeconfig
    </button>
  {/snippet}

  {#snippet headerSuffix()}
    <div></div>
  {/snippet}

  {#snippet cell({ item: ctx, column })}
    {@const value = getCellValue(ctx, column.name)}
    {#if column.name === "Status"}
      <span class="px-1.5 py-0.5 text-xs rounded border {statusBadgeClass(value)}">{value}</span>
    {:else}
      <span>{value}</span>
    {/if}
  {/snippet}

  {#snippet rowSuffix({ item: ctx })}
    {@const status = clusterStore.connectionStatus[ctx.name] ?? "disconnected"}
    <div class="flex items-center justify-end gap-1">
      <button
        type="button"
        onclick={(e) => { e.stopPropagation(); push(`/settings/clusters/${encodeURIComponent(ctx.name)}`) }}
        class="p-1 rounded text-muted hover:text-fg hover:bg-surface-hover transition-colors"
        title="Cluster settings"
        aria-label="Settings for {ctx.name}"
      >
        <Settings size={13} />
      </button>
      {#if status !== "disconnected"}
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); clusterStore.disconnect(ctx.name) }}
          class="p-1 rounded opacity-0 group-hover:opacity-60 hover:!opacity-100 hover:text-destructive transition-all"
          title="Disconnect"
          aria-label="Disconnect {ctx.name}"
        >
          <Unplug size={13} />
        </button>
      {/if}
    </div>
  {/snippet}
</DataTable>
