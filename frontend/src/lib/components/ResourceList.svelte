<script lang="ts">
  import {Trash2, RefreshCw, Columns3, Check, Minus, Download} from "lucide-svelte";
  import {ConfirmDialog} from "@klados/ui";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {evalExpr, defaultAlign, type ColumnDef, type RenderType} from "$lib/registry/index";
  import {getControllerRef, type ControllerRef} from "$lib/utils/relationships";
  import {DeleteResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import {formatAge} from "$lib/utils/age";
  import {onMount, untrack} from "svelte";
  import {slotRegistry} from "$lib/plugins/slots.svelte.js";
  import {loadPluginComponent} from "$lib/plugins/loader.js";
  import {streamingStore} from "$lib/stores/streaming.svelte.js";
  import Sparkline from "./charts/Sparkline.svelte";
  import type {MetricResult} from "./charts/types";
  import {columnStore} from "$lib/stores/columns.svelte";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {selectionStore} from "$lib/stores/selection.svelte";
  import ColumnPicker from "./ColumnPicker.svelte";
  import ViewOptionsMenu from "./ViewOptionsMenu.svelte";
  import {Eye} from "lucide-svelte";
  import {itemToYaml} from "$lib/utils/yamlClipboard";
  import {bottomPanelStore} from "$lib/stores/bottom-panel.svelte";
  import DataTable, {type DataTableColumn} from "./DataTable.svelte";
  import SmartSearch from "./SmartSearch.svelte";
  import SavedFilterDropdown from "./SavedFilterDropdown.svelte";
  import {filterItems} from "$lib/search/filter";
  import type {SearchTerm} from "$lib/search/parser";
  import {exportItems} from "$lib/utils/export";
  import type {KubernetesResource} from "$lib/types";
  import {shortcutStore} from "$lib/stores/shortcuts.svelte";
  import {shortcutActions} from "$lib/stores/shortcutActions.svelte";
  import HealthBadge from "./HealthBadge.svelte";
  import {volumeBrowserStore} from "$lib/stores/volumeBrowser.svelte";

  function itemKey(obj: KubernetesResource): string {
    const ns = obj.metadata?.namespace ?? "";
    const name = obj.metadata?.name ?? "";
    return ns ? `${ns}/${name}` : name;
  }

  let now = $state(Date.now());
  onMount(() => {
    const id = setInterval(() => {
      now = Date.now();
    }, 1000);
    return () => clearInterval(id);
  });

  onMount(() => {
    shortcutStore.register({
      id: "delete-selected",
      keys: "Delete",
      description: "Delete selected resources",
      category: "Resources",
      action: () => {
        shortcutActions.deleteSelected++;
      },
    });
    shortcutStore.register({
      id: "select-all",
      keys: "Control+a",
      description: "Select / deselect all",
      category: "Resources",
      action: () => {
        shortcutActions.selectAll++;
      },
    });
    shortcutStore.register({
      id: "copy-resource-names",
      keys: "Control+Shift+C",
      description: "Copy selected resource names",
      category: "Resources",
      action: () => {
        shortcutActions.copyResourceNames++;
      },
    });
    return () => {
      shortcutStore.unregister("delete-selected");
      shortcutStore.unregister("select-all");
      shortcutStore.unregister("copy-resource-names");
    };
  });

  let {
    items,
    contextName,
    gvr,
    selectedNamespaces = [],
    loading = false,
    error = null,
    selectedName = null,
    scrollContainer = $bindable<HTMLDivElement | undefined>(undefined),
    onrefresh,
    onselect,
    onopenowner,
    sparklineGvrs = [],
    sparklineData = {},
    sparklineColumns = [],
    onSparklineToggle,
    rowActions,
  }: {
    items: Record<string, KubernetesResource>[];
    contextName: string;
    gvr: string;
    selectedNamespaces?: string[];
    loading?: boolean;
    error?: string | null;
    selectedName?: string | null;
    scrollContainer?: HTMLDivElement;
    onrefresh?: () => void;
    onselect?: (item: Record<string, KubernetesResource>) => void;
    onopenowner?: (ref: ControllerRef, namespace: string) => void;
    sparklineGvrs?: string[];
    sparklineData?: Record<string, MetricResult[]>;
    sparklineColumns?: string[];
    onSparklineToggle?: (columns: string[]) => void;
    rowActions?: (
      item: Record<string, KubernetesResource>,
    ) => Array<{label: string; icon?: KubernetesResource; onClick: () => void; variant?: "default" | "destructive"}>;
  } = $props();

  let searchTerms = $state<SearchTerm[]>([]);
  let searchQuery = $state("");
  let deleteTarget = $state<{namespace: string; name: string} | null>(null);
  let confirmOpen = $state(false);
  let ctxMenu = $state<{x: number; y: number; item: Record<string, KubernetesResource>} | null>(null);
  let ctxMenuEl = $state<HTMLDivElement | null>(null);
  let columnMenuOpen = $state(false);
  let viewMenuOpen = $state(false);
  let exportMenuOpen = $state(false);

  const POD_OWNER_GVRS = new Set([
    "apps.v1.deployments",
    "apps.v1.statefulsets",
    "apps.v1.daemonsets",
    "apps.v1.replicasets",
    "batch.v1.jobs",
    "batch.v1.cronjobs",
  ]);

  const isPod = $derived(gvr === "core.v1.pods");
  const isPodOwner = $derived(POD_OWNER_GVRS.has(gvr));
  const canViewLogs = $derived(isPod || isPodOwner);
  const canOpenTerminal = $derived(isPod && clusterStore.canMutate());

  function getSparklinePoints(itemName: string, metricName: string): {t: number; v: number}[] {
    const metrics = sparklineData[itemName];
    if (!metrics) {
      return [];
    }
    const metric = metrics.find((m) => m.name === metricName);
    if (!metric?.series?.[0]?.points) {
      return [];
    }
    return metric.series[0].points;
  }

  const pluginColumns = $derived(slotRegistry.getListColumns(gvr));
  const pluginMenuItems = $derived(slotRegistry.getContextMenuItems(gvr));
  const basePluginURL = $derived(streamingStore.config ? streamingStore.pluginsBaseUrl() : null);

  $effect(() => {
    if (!ctxMenu) {
      return;
    }
    const close = () => {
      ctxMenu = null;
    };
    window.addEventListener("click", close, {once: true});
    return () => window.removeEventListener("click", close);
  });

  $effect(() => {
    if (!(ctxMenu && ctxMenuEl)) {
      return;
    }
    const rect = ctxMenuEl.getBoundingClientRect();
    const maxX = window.innerWidth - rect.width - 8;
    const maxY = window.innerHeight - rect.height - 8;
    if (ctxMenu.x > maxX || ctxMenu.y > maxY) {
      ctxMenu = {
        ...ctxMenu,
        x: Math.max(0, Math.min(ctxMenu.x, maxX)),
        y: Math.max(0, Math.min(ctxMenu.y, maxY)),
      };
    }
  });

  $effect(() => {
    if (!columnMenuOpen) {
      return;
    }
    const close = () => {
      columnMenuOpen = false;
    };
    const timer = setTimeout(() => window.addEventListener("click", close, {once: true}), 0);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("click", close);
    };
  });

  $effect(() => {
    if (!viewMenuOpen) {
      return;
    }
    const close = () => {
      viewMenuOpen = false;
    };
    const timer = setTimeout(() => window.addEventListener("click", close, {once: true}), 0);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("click", close);
    };
  });

  $effect(() => {
    if (!exportMenuOpen) {
      return;
    }
    const close = () => {
      exportMenuOpen = false;
    };
    const timer = setTimeout(() => window.addEventListener("click", close, {once: true}), 0);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("click", close);
    };
  });

  // Scroll to top when GVR changes
  $effect(() => {
    void gvr;
    searchQuery = "";
    searchTerms = [];
    if (scrollContainer) {
      scrollContainer.scrollTop = 0;
    }
  });

  const filtered = $derived.by(() => {
    let result = items;
    if (selectedNamespaces.length > 1) {
      result = result.filter((item) => selectedNamespaces.includes(item.metadata?.namespace ?? ""));
    }
    result = filterItems(result, searchTerms);
    if (columnStore.sortState) {
      const {column, direction} = columnStore.sortState;
      const col = columnStore.visibleColumns.find((c) => c.name === column);
      if (col?.expr) {
        const keyed = result.map((item) => ({item, key: String(evalExpr(col.expr, item) ?? "")}));
        const isAge = col.renderType === "age";
        keyed.sort((a, b) => {
          let cmp: number;
          if (isAge) {
            cmp = a.key.localeCompare(b.key);
          } else {
            const an = Number.parseFloat(a.key);
            const bn = Number.parseFloat(b.key);
            cmp = Number.isFinite(an) && Number.isFinite(bn) ? an - bn : a.key.localeCompare(b.key);
          }
          return direction === "asc" ? cmp : -cmp;
        });
        result = keyed.map((k) => k.item);
      }
    }
    return result;
  });

  const filteredKeys = $derived(filtered.map((item) => itemKey(item)));
  const filteredItemsByKey = $derived.by(() => {
    const map = new Map<string, Record<string, KubernetesResource>>();
    for (const item of filtered) {
      map.set(itemKey(item), item);
    }
    return map;
  });

  $effect(() => {
    selectionStore.setVisibleKeys(new Set(filteredKeys));
  });

  const allVisibleSelected = $derived(filtered.length > 0 && filteredKeys.every((k) => selectionStore.isSelected(k)));
  const someVisibleSelected = $derived(!allVisibleSelected && filteredKeys.some((k) => selectionStore.isSelected(k)));
  const canMutate = $derived(clusterStore.canMutate());

  // Action bus: select all / deselect all
  $effect(() => {
    shortcutActions.selectAll;
    if (shortcutActions.selectAll > 0) {
      untrack(() => {
        if (allVisibleSelected) {
          selectionStore.deselectAll();
        } else {
          selectionStore.selectAll(filteredKeys, filteredItemsByKey);
        }
      });
    }
  });

  // Action bus: refresh
  $effect(() => {
    shortcutActions.refreshList;
    if (shortcutActions.refreshList > 0) {
      onrefresh?.();
    }
  });

  // Action bus: copy resource names
  $effect(() => {
    shortcutActions.copyResourceNames;
    if (shortcutActions.copyResourceNames > 0 && selectionStore.count > 0) {
      const names = selectionStore.items()
        .map((item) => {
          const meta = item.metadata as Record<string, unknown> | undefined;
          return (meta?.name as string) ?? "";
        })
        .filter(Boolean);
      if (names.length > 0) {
        navigator.clipboard.writeText(names.join("\n")).then(() => {
          notificationStore.push(`Copied ${names.length} name${names.length > 1 ? "s" : ""}`, "info");
        });
      }
    }
  });

  const tooManyForSparklines = $derived(filtered.length > 200);

  const hasActiveFilters = $derived(searchTerms.length > 0);
  const emptyMessageText = $derived(hasActiveFilters ? "No resources match these filters" : "No resources found");

  // Map ColumnDef[] to DataTableColumn[] with computed alignment
  const dataTableColumns = $derived<DataTableColumn[]>(
    columnStore.visibleColumns.map((c) => ({
      name: c.name,
      width: c.width,
      align: c.align ?? defaultAlign(c.renderType),
    })),
  );

  // Look up the full ColumnDef by name for cell rendering
  function getColumnDef(name: string): ColumnDef | undefined {
    return columnStore.visibleColumns.find((c) => c.name === name);
  }

  function renderCell(col: ColumnDef, item: Record<string, KubernetesResource>) {
    return evalExpr(col.expr, item);
  }

  function renderValue(value: KubernetesResource, renderType: RenderType): string {
    if (value == null) {
      return "";
    }
    if (renderType === "age") {
      return formatAge(String(value), now);
    }
    return String(value);
  }

  function badgeClass(value: KubernetesResource): string {
    const v = String(value ?? "").toLowerCase();
    if (["running", "active", "bound", "available", "true"].includes(v)) {
      return "bg-accent/20 text-accent border-accent/30";
    }
    if (["error", "crashloopbackoff", "failed", "oomkilled"].includes(v)) {
      return "bg-destructive/20 text-destructive border-destructive/30";
    }
    if (["pending", "terminating"].includes(v)) {
      return "bg-muted/20 text-muted border-muted/30";
    }
    return "bg-muted/10 text-fg border-border";
  }

  async function confirmDelete() {
    if (!deleteTarget) {
      return;
    }
    const {namespace, name} = deleteTarget;
    try {
      await DeleteResource(contextName, gvr, namespace, name);
      notificationStore.push(`Deleted ${name}`, "success");
    } catch (e: unknown) {
      notificationStore.push(`Failed to delete: ${(e as {message?: string})?.message ?? String(e)}`, "error");
    }
    deleteTarget = null;
  }

  function requestDelete(item: Record<string, KubernetesResource>) {
    deleteTarget = {
      namespace: item.metadata?.namespace ?? "",
      name: item.metadata?.name ?? "",
    };
    confirmOpen = true;
  }

  const prefixGridCols = $derived(canMutate ? ["36px"] : []);

  const suffixGridCols = $derived.by(() => {
    const parts: string[] = [];
    for (const _ of pluginColumns) {
      parts.push("1fr");
    }
    for (const _ of sparklineColumns) {
      parts.push("80px");
    }
    parts.push("36px");
    return parts;
  });
</script>

<DataTable
  items={filtered}
  visibleColumns={dataTableColumns}
  pinnedNames={columnStore.pinnedNames()}
  sortState={columnStore.sortState}
  compact={columnStore.compact}
  {loading}
  {error}
  emptyMessage={emptyMessageText}
  {prefixGridCols}
  {suffixGridCols}
  bind:scrollContainer
  selectedRow={(item) => selectedName === `${item.metadata?.name ?? ''}/${item.metadata?.namespace ?? ''}`}
  onsort={(col, dir) => columnStore.setSort(col, dir)}
  onresize={(col, width) => columnStore.resizeColumn(col, width)}
  onreorder={(names) => columnStore.reorderVisible(names)}
  onTogglePin={(name) => columnStore.setPinned(name, !columnStore.isPinned(name))}
  onHideColumn={(name) => columnStore.setColumnVisible(name, false)}
  onrowclick={onselect ? (item) => onselect?.(item) : undefined}
  oncontextmenu={(e, item) => { ctxMenu = { x: e.clientX, y: e.clientY, item } }}
>
  {#snippet toolbar()}
    <SmartSearch {items} bind:value={searchQuery} ontermschange={(t) => { searchTerms = t }} />
    <SavedFilterDropdown {gvr} {contextName} currentQuery={searchQuery} onapply={(q) => { searchQuery = q }} />
    <span class="text-xs text-muted">{filtered.length} items</span>
    <div class="relative">
      <button
        type="button"
        onclick={() => exportMenuOpen = !exportMenuOpen}
        class="p-1 rounded hover:bg-surface-hover transition-colors"
        title="Export visible"
        aria-label="Export visible"
      >
        <Download size={14} />
      </button>
      {#if exportMenuOpen}
        <div class="absolute top-full mt-1 right-0 z-50 bg-surface border border-border rounded shadow-lg py-1 min-w-24">
          <button
            type="button"
            class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
            onclick={() => { exportItems(filtered, gvr, 'yaml'); exportMenuOpen = false }}
          >
            YAML
          </button>
          <button
            type="button"
            class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
            onclick={() => { exportItems(filtered, gvr, 'json'); exportMenuOpen = false }}
          >
            JSON
          </button>
        </div>
      {/if}
    </div>
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
        <ColumnPicker
          visibleColumns={columnStore.visibleColumns}
          allColumns={columnStore.allColumns}
          pinnedNames={columnStore.pinnedNames()}
          onToggle={(name, visible) => columnStore.setColumnVisible(name, visible)}
          onReset={() => columnStore.reset()}
        />
      {/if}
    </div>
    <div class="relative">
      <button
        type="button"
        onclick={() => viewMenuOpen = !viewMenuOpen}
        class="p-1 rounded hover:bg-surface-hover transition-colors"
        title="View options"
        aria-label="View options"
      >
        <Eye size={14} />
      </button>
      {#if viewMenuOpen}
        <ViewOptionsMenu
          compact={columnStore.compact}
          onCompactChange={(v) => columnStore.setCompact(v)}
          hasSparklines={sparklineGvrs.includes(gvr)}
          {sparklineColumns}
          {onSparklineToggle}
        />
      {/if}
    </div>
    {#if onrefresh}
      <button
        type="button"
        onclick={onrefresh}
        class="p-1 rounded hover:bg-surface-hover transition-colors"
        title="Refresh"
        aria-label="Refresh"
      >
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
      </button>
    {/if}
  {/snippet}

  {#snippet emptyAction()}
    {#if hasActiveFilters}
      <button
        type="button"
        onclick={() => { searchQuery = ''; searchTerms = []; }}
        class="px-3 py-1.5 text-sm border border-border rounded hover:bg-surface-hover transition-colors"
      >
        Clear filters
      </button>
    {/if}
  {/snippet}

  {#snippet headerPrefix()}
    {#if canMutate}
      <div class="flex items-center justify-center {columnStore.compact ? 'py-1' : 'py-2'}">
        <button
          type="button"
          onclick={() => {
            if (allVisibleSelected) {
              selectionStore.deselectAll()
            } else {
              selectionStore.selectAll(filteredKeys, filteredItemsByKey)
            }
          }}
          class="w-4 h-4 rounded border border-border flex items-center justify-center hover:border-accent transition-colors
            {allVisibleSelected || someVisibleSelected ? 'bg-accent border-accent' : ''}"
          aria-label={allVisibleSelected ? 'Deselect all' : 'Select all'}
        >
          {#if allVisibleSelected}
            <Check size={10} class="text-accent-fg" />
          {:else if someVisibleSelected}
            <Minus size={10} class="text-accent-fg" />
          {/if}
        </button>
      </div>
    {/if}
  {/snippet}

  {#snippet headerSuffix()}
    {#each pluginColumns as pcol (pcol.id)}
      <div class="{columnStore.compact ? 'py-1' : 'py-2'} px-1">{pcol.label}</div>
    {/each}
    {#each sparklineColumns as scol}
      <div class="{columnStore.compact ? 'py-1' : 'py-2'} px-1">{scol}</div>
    {/each}
    <div></div>
  {/snippet}

  {#snippet rowPrefix({ item })}
    {#if canMutate}
      {@const key = itemKey(item)}
      <div class="flex items-center justify-center" onclick={(e) => e.stopPropagation()} role="none">
        <button
          type="button"
          onclick={(e) => {
            e.stopPropagation()
            if (e.shiftKey) {
              selectionStore.selectRange(key, filteredKeys, filteredItemsByKey)
            } else {
              selectionStore.toggle(key, item)
            }
          }}
          class="w-4 h-4 rounded border border-border flex items-center justify-center hover:border-accent transition-colors
            {selectionStore.isSelected(key) ? 'bg-accent border-accent' : ''}"
          aria-label={selectionStore.isSelected(key) ? 'Deselect' : 'Select'}
        >
          {#if selectionStore.isSelected(key)}
            <Check size={10} class="text-accent-fg" />
          {/if}
        </button>
      </div>
    {/if}
  {/snippet}

  {#snippet cell({ item, column })}
    {@const col = getColumnDef(column.name)}
    {#if col}
      {@const value = renderCell(col, item)}
      {#if col.name === 'Name'}
        <span class="flex items-center gap-1.5 truncate" title={renderValue(value, col.renderType)}><HealthBadge obj={item} />{renderValue(value, col.renderType)}</span>
      {:else if col.name === 'Namespace'}
        <button
          type="button"
          class="hover:text-accent cursor-pointer truncate text-left"
          onclick={(e) => { e.stopPropagation(); clusterStore.setNamespaces(contextName, [String(value)]) }}
        >
          {renderValue(value, col.renderType)}
        </button>
      {:else if col.renderType === 'controlledBy'}
        {@const ref = getControllerRef(item)}
        {#if ref}
          {#if onopenowner && clusterStore.resolveOwnerGVR(ref.apiVersion, ref.kind)}
            <button
              type="button"
              class="text-accent hover:underline cursor-pointer"
              title="{ref.kind}/{ref.name}"
              onclick={(e) => { e.stopPropagation(); onopenowner?.(ref, item.metadata?.namespace ?? '') }}
            >
              {ref.kind}
            </button>
          {:else}
            <span title="{ref.kind}/{ref.name}">{ref.kind}</span>
          {/if}
        {/if}
      {:else if col.renderType === 'badge'}
        <span class="px-1.5 py-0.5 text-xs rounded border {badgeClass(value)}" title={renderValue(value, col.renderType)}>
          {renderValue(value, col.renderType)}
        </span>
      {:else}
        <span class={col.renderType === 'age' ? 'text-muted' : ''} title={renderValue(value, col.renderType)}>
          {renderValue(value, col.renderType)}
        </span>
      {/if}
    {/if}
  {/snippet}

  {#snippet rowSuffix({ item })}
    {#each pluginColumns as pcol (pcol.id)}
      <div class="px-1 flex items-center overflow-hidden text-sm">
        {#if basePluginURL}
          {#await loadPluginComponent(pcol.pluginName, pcol.component, basePluginURL) then Cmp}
            {#if Cmp}
              <Cmp resource={item} />
            {/if}
          {/await}
        {/if}
      </div>
    {/each}
    {#each sparklineColumns as scol}
      <div class="px-1 flex items-center overflow-hidden">
        {#if tooManyForSparklines}
          <span class="text-xs text-muted" title="Sparklines disabled for >200 resources">Too many</span>
        {:else}
          {@const pts = getSparklinePoints(item.metadata?.name ?? '', scol)}
          {#if pts.length > 0}
            <Sparkline points={pts} height={20} />
          {:else}
            <div style="height: 20px;"></div>
          {/if}
        {/if}
      </div>
    {/each}
    <div class="flex items-center justify-end gap-1">
      {#if rowActions}
        {#each rowActions(item) as action}
          <button
            type="button"
            onclick={(e) => { e.stopPropagation(); action.onClick() }}
            class="p-1 rounded opacity-0 group-hover:opacity-60 hover:!opacity-100 transition-all {action.variant === 'destructive' ? 'hover:text-destructive' : 'hover:text-fg'}"
            title={action.label}
            aria-label={action.label}
          >
            {#if action.icon}
              <action.icon size={13} />
            {:else}
              <span class="text-xs">{action.label}</span>
            {/if}
          </button>
        {/each}
      {:else if canMutate}
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); requestDelete(item) }}
          class="p-1 rounded opacity-0 group-hover:opacity-60 hover:!opacity-100 hover:text-destructive transition-all"
          title="Delete"
          aria-label="Delete {item.metadata?.name}"
        >
          <Trash2 size={13} />
        </button>
      {/if}
    </div>
  {/snippet}
</DataTable>

{#if ctxMenu}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    bind:this={ctxMenuEl}
    class="fixed z-50 bg-surface border border-border rounded shadow-lg py-1 min-w-36"
    style="left:{ctxMenu.x}px; top:{ctxMenu.y}px"
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => e.stopPropagation()}
  >
    <button
      type="button"
      class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
      onclick={() => {
        if (ctxMenu) {
          const name = ctxMenu.item.metadata?.name as string | undefined;
          if (name) {
            navigator.clipboard.writeText(name);
            notificationStore.push("Copied name", "info");
          }
          ctxMenu = null;
        }
      }}
    >Copy name</button>
    <button
      type="button"
      class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
      onclick={() => {
        if (ctxMenu) {
          navigator.clipboard.writeText(itemToYaml(ctxMenu.item as unknown as Record<string, unknown>));
          notificationStore.push("Copied YAML", "info");
          ctxMenu = null;
        }
      }}
    >Copy YAML</button>
    <button
      type="button"
      class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
      onclick={() => {
        if (ctxMenu) {
          const item = ctxMenu.item;
          bottomPanelStore.addTab({
            kind: "yaml",
            resourceKind: (item.kind as unknown as string) ?? "",
            resourceName: (item.metadata?.name as unknown as string) ?? "",
            ctxName: contextName,
            gvr,
            namespace: (item.metadata?.namespace as unknown as string) ?? "",
            name: (item.metadata?.name as unknown as string) ?? "",
            obj: item as unknown as Record<string, unknown>,
          });
          ctxMenu = null;
        }
      }}
    >View YAML</button>
    {#if canViewLogs}
      <div class="border-t border-border my-1"></div>
      <button
        type="button"
        class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
        onclick={() => {
          if (ctxMenu) {
            const item = ctxMenu.item;
            bottomPanelStore.addTab({
              kind: isPod ? "logs" : "aggregate-logs",
              resourceKind: (item.kind as unknown as string) ?? "",
              resourceName: (item.metadata?.name as unknown as string) ?? "",
              ctxName: contextName,
              gvr,
              namespace: (item.metadata?.namespace as unknown as string) ?? "",
              name: (item.metadata?.name as unknown as string) ?? "",
              obj: item as unknown as Record<string, unknown>,
            });
            ctxMenu = null;
          }
        }}
      >View logs</button>
    {/if}
    {#if canOpenTerminal}
      <button
        type="button"
        class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
        onclick={() => {
          if (ctxMenu) {
            const item = ctxMenu.item;
            bottomPanelStore.addTab({
              kind: "terminal",
              resourceKind: (item.kind as unknown as string) ?? "",
              resourceName: (item.metadata?.name as unknown as string) ?? "",
              ctxName: contextName,
              gvr,
              namespace: (item.metadata?.namespace as unknown as string) ?? "",
              name: (item.metadata?.name as unknown as string) ?? "",
              obj: item as unknown as Record<string, unknown>,
            });
            ctxMenu = null;
          }
        }}
      >Open terminal</button>
    {/if}
    <div class="border-t border-border my-1"></div>
    {#each pluginMenuItems as mi (mi.id)}
      {#if basePluginURL}
        {#await loadPluginComponent(mi.pluginName, mi.component, basePluginURL) then Cmp}
          {#if Cmp}
            {@const menuItem = ctxMenu}
            <Cmp resource={menuItem.item} onclose={() => { ctxMenu = null }} />
          {/if}
        {/await}
      {/if}
    {/each}
    {#if canMutate && gvr === 'core.v1.persistentvolumeclaims'}
      <button
        type="button"
        class="w-full text-left px-3 py-1.5 text-sm hover:bg-surface-hover"
        onclick={(e) => {
          if (ctxMenu) {
            const item = ctxMenu.item;
            const ns = (item.metadata as { namespace?: string } | undefined)?.namespace ?? ''
            const nm = (item.metadata as { name?: string } | undefined)?.name ?? ''
            ctxMenu = null
            void volumeBrowserStore.spawn(contextName, ns, nm, { shiftHeld: e.shiftKey })
          }
        }}
      >
        Browse Volume
      </button>
    {/if}
    {#if canMutate}
      <button
        type="button"
        class="w-full text-left px-3 py-1.5 text-sm text-destructive hover:bg-surface-hover"
        onclick={() => { if (ctxMenu) { requestDelete(ctxMenu.item); ctxMenu = null } }}
      >
        Delete
      </button>
    {/if}
  </div>
{/if}

<ConfirmDialog
  bind:open={confirmOpen}
  title="Delete resource"
  message="Delete {deleteTarget?.name}? This action cannot be undone."
  confirmLabel="Delete"
  onconfirm={confirmDelete}
/>
