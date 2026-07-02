<script lang="ts">
  import {DetailDrawer} from "@klados/ui";
  import {createResourceStore} from "$lib/stores/resource.svelte";
  import {clusterStore} from "$lib/stores/cluster.svelte";
  import {descriptorRegistry} from "$lib/registry/index";
  import {registryLoaded} from "$lib/registry/loaded.svelte";
  import {columnStore} from "$lib/stores/columns.svelte";
  import {notificationStore} from "$lib/stores/notification.svelte";
  import {GetResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
  import DataTable, {type DataTableColumn} from "$lib/components/DataTable.svelte";
  import ColumnMenu from "$lib/components/ColumnMenu.svelte";
  import ResourceDetail from "$lib/components/ResourceDetail.svelte";
  import EventToolbar from "$lib/components/events/EventToolbar.svelte";
  import EventSeverityTimeline from "$lib/components/events/EventSeverityTimeline.svelte";
  import EventDetailPanel from "$lib/components/events/EventDetailPanel.svelte";
  import {
    classifySeverity,
    rowReason,
    rowMessage,
    rowCount,
    rowLastSeen,
    rowFirstSeen,
    rowInvolvedObject,
    rowSample,
    formatObject,
    eventTimestamp,
  } from "$lib/event/event-columns";
  import {groupEvents} from "$lib/event/event-grouping";
  import {isGrouped, type EventItem, type EventRow, type InvolvedObjectRef} from "$lib/event/event-types";
  import {formatAge} from "$lib/utils/age";
  import type {ControllerRef} from "$lib/utils/relationships";
  import EventTypeBadge from "$lib/event/EventTypeBadge.svelte";

  let {params = {}}: {params?: Record<string, string>} = $props();

  const ctxName = $derived(params.ctx ?? "");
  const EVENTS_GVR = "core.v1.events";

  $effect(() => { if (ctxName) clusterStore.setActiveContext(ctxName); });
  $effect(() => { columnStore.loadForGVR(EVENTS_GVR); });

  const store = createResourceStore();
  const selectedNs = $derived(clusterStore.getSelectedNamespaces(ctxName));
  const watchNamespace = $derived(selectedNs.length === 1 ? selectedNs[0] : "");

  $effect(() => {
    if (ctxName) store.start(ctxName, EVENTS_GVR, watchNamespace);
    return () => store.stop();
  });

  let showWarning = $state(true);
  let showNormal = $state(true);
  let selectedKinds = $state<string[]>([]);
  let selectedReasons = $state<string[]>([]);
  let search = $state("");
  let grouped = $state(false);
  let timeWindow = $state<{from: number; to: number} | null>(null);
  const rangeMs = 30 * 60_000;

  let listScrollContainer = $state<HTMLDivElement | undefined>();
  let paused = $state(false);

  let selectedEvent = $state<EventRow | null>(null);
  let selectedInvolvedItem = $state<Record<string, unknown> | null>(null);
  let selectedInvolvedGVR = $state<string>("");

  let columnMenuOpen = $state(false);

  let now = $state(Date.now());
  $effect(() => {
    const id = setInterval(() => { now = Date.now(); }, 1000);
    return () => clearInterval(id);
  });

  $effect(() => {
    if (!columnMenuOpen) return;
    const close = () => { columnMenuOpen = false; };
    const timer = setTimeout(() => window.addEventListener("click", close, {once: true}), 0);
    return () => { clearTimeout(timer); window.removeEventListener("click", close); };
  });

  const rawItems = $derived(store.items as unknown as EventItem[]);

  const nsFiltered = $derived(
    selectedNs.length > 0
      ? rawItems.filter((e) => selectedNs.includes(e.metadata?.namespace ?? ""))
      : rawItems,
  );
  const severityFiltered = $derived(
    nsFiltered.filter((e) => {
      const sev = classifySeverity(e);
      return sev === "Warning" ? showWarning : showNormal;
    }),
  );
  const kindFiltered = $derived(
    selectedKinds.length === 0
      ? severityFiltered
      : severityFiltered.filter((e) => selectedKinds.includes(e.involvedObject?.kind ?? "")),
  );
  const reasonFiltered = $derived(
    selectedReasons.length === 0
      ? kindFiltered
      : kindFiltered.filter((e) => selectedReasons.includes(e.reason ?? "")),
  );
  const searchFiltered = $derived.by(() => {
    if (!search.trim()) return reasonFiltered;
    const needle = search.toLowerCase();
    return reasonFiltered.filter((e) => {
      return (
        (e.reason ?? "").toLowerCase().includes(needle) ||
        (e.message ?? "").toLowerCase().includes(needle) ||
        (e.involvedObject?.name ?? "").toLowerCase().includes(needle)
      );
    });
  });
  const windowFiltered = $derived.by(() => {
    if (!timeWindow) return searchFiltered;
    return searchFiltered.filter((e) => {
      const ts = Date.parse(eventTimestamp(e));
      return Number.isFinite(ts) && ts >= timeWindow!.from && ts < timeWindow!.to;
    });
  });

  function sortKey(row: EventRow, columnName: string): {kind: "string" | "number"; value: string | number} {
    switch (columnName) {
      case "Type":       return {kind: "string", value: classifySeverity(row)};
      case "Reason":     return {kind: "string", value: rowReason(row)};
      case "Object":     return {kind: "string", value: formatObject(rowInvolvedObject(row))};
      case "Message":    return {kind: "string", value: rowMessage(row)};
      case "Count":      return {kind: "number", value: rowCount(row)};
      case "First seen": return {kind: "string", value: rowFirstSeen(row)};
      case "Last seen":  return {kind: "string", value: rowLastSeen(row)};
      case "Namespace":  return {kind: "string", value: rowInvolvedObject(row).namespace};
      case "Source":     return {kind: "string", value: rowSample(row).source?.component ?? ""};
      default:           return {kind: "string", value: ""};
    }
  }

  const sortedItems = $derived.by(() => {
    const copy = [...windowFiltered];
    const sortState = columnStore.sortState;
    if (sortState) {
      const dir = sortState.direction === "asc" ? 1 : -1;
      copy.sort((a, b) => {
        const ka = sortKey(a as EventRow, sortState.column);
        const kb = sortKey(b as EventRow, sortState.column);
        let cmp: number;
        if (ka.kind === "number") {
          cmp = (ka.value as number) - (kb.value as number);
        } else {
          cmp = String(ka.value).localeCompare(String(kb.value));
        }
        if (cmp === 0) {
          cmp = (a.metadata?.uid ?? "").localeCompare(b.metadata?.uid ?? "");
        }
        return cmp * dir;
      });
    } else {
      copy.sort((a, b) => {
        const ta = eventTimestamp(a);
        const tb = eventTimestamp(b);
        if (ta !== tb) return tb.localeCompare(ta);
        return (a.metadata?.uid ?? "").localeCompare(b.metadata?.uid ?? "");
      });
    }
    return copy;
  });

  const rows: EventRow[] = $derived(
    grouped ? (groupEvents(sortedItems) as EventRow[]) : (sortedItems as EventRow[]),
  );

  const availableKinds = $derived.by(() => {
    const s = new Set<string>();
    for (const e of rawItems) {
      const k = e.involvedObject?.kind;
      if (k) s.add(k);
    }
    return Array.from(s).sort();
  });
  const availableReasons = $derived.by(() => {
    const s = new Set<string>();
    for (const e of rawItems) {
      if (e.reason) s.add(e.reason);
    }
    return Array.from(s).sort();
  });

  const _kindFilteredAll = $derived(
    selectedKinds.length === 0
      ? nsFiltered
      : nsFiltered.filter((e) => selectedKinds.includes(e.involvedObject?.kind ?? "")),
  );
  const _reasonFilteredAll = $derived(
    selectedReasons.length === 0
      ? _kindFilteredAll
      : _kindFilteredAll.filter((e) => selectedReasons.includes(e.reason ?? "")),
  );
  const _searchFilteredAll = $derived.by(() => {
    if (!search.trim()) return _reasonFilteredAll;
    const needle = search.toLowerCase();
    return _reasonFilteredAll.filter((e) =>
      (e.reason ?? "").toLowerCase().includes(needle) ||
      (e.message ?? "").toLowerCase().includes(needle) ||
      (e.involvedObject?.name ?? "").toLowerCase().includes(needle)
    );
  });
  const _windowFilteredAll = $derived.by(() => {
    if (!timeWindow) return _searchFilteredAll;
    return _searchFilteredAll.filter((e) => {
      const ts = Date.parse(eventTimestamp(e));
      return Number.isFinite(ts) && ts >= timeWindow!.from && ts < timeWindow!.to;
    });
  });

  const warningCount = $derived(_windowFilteredAll.filter((e) => classifySeverity(e) === "Warning").length);

  const dataTableColumns = $derived<DataTableColumn[]>(
    columnStore.visibleColumns.map((c) => ({
      name: c.name,
      width: c.width,
    })),
  );

  function cellText(row: EventRow, name: string): string {
    switch (name) {
      case "Reason":     return rowReason(row);
      case "Object":     return formatObject(rowInvolvedObject(row));
      case "Message":    return rowMessage(row);
      case "Count":      return String(rowCount(row));
      case "Namespace":  return rowInvolvedObject(row).namespace;
      case "Source":     return rowSample(row).source?.component ?? "";
      default:           return "";
    }
  }

  function rowKey(row: EventRow): string {
    if (isGrouped(row)) return row.key;
    return (row as EventItem).metadata?.uid ?? `${(row as EventItem).reason ?? ""}/${(row as EventItem).metadata?.name ?? ""}`;
  }

  function openEvent(row: EventRow) {
    selectedEvent = row;
    selectedInvolvedItem = null;
    selectedInvolvedGVR = "";
  }
  async function openInvolvedObject(ref: InvolvedObjectRef, gvr: string) {
    try {
      const obj = await GetResource(ctxName, gvr, ref.namespace, ref.name);
      if (obj) {
        selectedInvolvedItem = obj as Record<string, unknown>;
        selectedInvolvedGVR = gvr;
      }
    } catch {
      notificationStore.push("Involved object not found", "error");
    }
  }
  async function openOwnerDrawer(ref: ControllerRef, namespace: string) {
    const ownerGVR = clusterStore.resolveOwnerGVR(ref.apiVersion, ref.kind);
    if (!ownerGVR) return;
    try {
      const owner = await GetResource(ctxName, ownerGVR, namespace, ref.name);
      if (owner) {
        selectedInvolvedItem = owner as Record<string, unknown>;
        selectedInvolvedGVR = ownerGVR;
      }
    } catch {
      notificationStore.push("Owner resource not found", "error");
    }
  }
  function closeDrawer() {
    selectedEvent = null;
    selectedInvolvedItem = null;
    selectedInvolvedGVR = "";
  }

  function onScroll() {
    const el = listScrollContainer;
    if (!el) return;
    paused = el.scrollTop > 20;
  }
  $effect(() => {
    const el = listScrollContainer;
    if (!el) return;
    el.addEventListener("scroll", onScroll);
    return () => el.removeEventListener("scroll", onScroll);
  });
  function jumpToLatest() {
    listScrollContainer?.scrollTo({top: 0, behavior: "smooth"});
  }

  const drawerOpen = $derived(selectedEvent !== null || selectedInvolvedItem !== null);

  const drawerItem = $derived.by(() => {
    if (selectedInvolvedItem) return selectedInvolvedItem;
    if (selectedEvent) return rowSample(selectedEvent) as Record<string, unknown>;
    return null;
  });

  const drawerGVR = $derived(selectedInvolvedGVR || EVENTS_GVR);

  const selectedDescriptor = $derived(
    selectedInvolvedGVR && registryLoaded() ? descriptorRegistry.get(selectedInvolvedGVR) : null,
  );
</script>

<div class="flex flex-col h-full">
  <div class="shrink-0 px-4 py-3 border-b border-border flex items-center gap-2">
    <h1 class="text-sm font-semibold">Event Stream</h1>
    <span class="text-xs text-muted">{ctxName}</span>
  </div>

  <EventSeverityTimeline
    filteredItems={windowFiltered}
    allItems={rawItems}
    {rangeMs}
    {now}
    selectedWindow={timeWindow}
    onBrush={(w) => { timeWindow = w; }}
  />

  <EventToolbar
    {showWarning}
    {showNormal}
    onSeverityChange={({showWarning: w, showNormal: n}) => { showWarning = w; showNormal = n; }}
    {availableKinds}
    {selectedKinds}
    onKindsChange={(v) => { selectedKinds = v; }}
    {availableReasons}
    {selectedReasons}
    onReasonsChange={(v) => { selectedReasons = v; }}
    {search}
    onSearchChange={(v) => { search = v; }}
    {grouped}
    onGroupedChange={(v) => { grouped = v; }}
    {paused}
    onJumpToLatest={jumpToLatest}
    totalCount={windowFiltered.length}
    {warningCount}
    rangeLabel="last 30m"
    {columnMenuOpen}
    onColumnMenuToggle={() => { columnMenuOpen = !columnMenuOpen; }}
    {timeWindow}
    onClearTimeWindow={() => { timeWindow = null; }}
  />

  <div class="flex-1 overflow-hidden relative">
    <DataTable
      items={rows}
      visibleColumns={dataTableColumns}
      sortState={columnStore.sortState}
      compact={columnStore.compact}
      loading={store.loading}
      error={store.error}
      emptyMessage="No events found"
      bind:scrollContainer={listScrollContainer}
      selectedRow={(row) => selectedEvent !== null && rowKey(row as EventRow) === rowKey(selectedEvent)}
      onsort={(col, dir) => columnStore.setSort(col, dir)}
      onresize={(col, width) => columnStore.resizeColumn(col, width)}
      onrowclick={(row) => openEvent(row as EventRow)}
    >
      {#snippet cell({item, column})}
        {@const row = item as EventRow}
        {#if column.name === "Type"}
          <EventTypeBadge severity={classifySeverity(row)} />
        {:else if column.name === "First seen"}
          <span class="text-muted">{rowFirstSeen(row) ? formatAge(rowFirstSeen(row), now) : "—"}</span>
        {:else if column.name === "Last seen"}
          <span class="text-muted">{rowLastSeen(row) ? formatAge(rowLastSeen(row), now) : "—"}</span>
        {:else}
          <span title={cellText(row, column.name)}>{cellText(row, column.name)}</span>
        {/if}
      {/snippet}
    </DataTable>

    {#if columnMenuOpen}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="absolute top-2 right-2 z-50" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
        <ColumnMenu
          visibleColumns={columnStore.visibleColumns}
          allColumns={columnStore.allColumns}
          compact={columnStore.compact}
          onToggle={(name, visible) => columnStore.setColumnVisible(name, visible)}
          onMove={(name, dir) => columnStore.moveColumn(name, dir)}
          onReset={() => columnStore.reset()}
          onCompactChange={(v) => columnStore.setCompact(v)}
          gvr={EVENTS_GVR}
        />
      </div>
    {/if}

    {#if drawerOpen && drawerItem}
      <DetailDrawer
        item={drawerItem}
        {ctxName}
        gvr={drawerGVR}
        onclose={closeDrawer}
        onFetchResource={async (c, g, ns, n) => { try { return await GetResource(c, g, ns, n); } catch { return null; } }}
      >
        {#snippet children({obj, onrefresh})}
          {#if selectedInvolvedItem && selectedDescriptor}
            <ResourceDetail
              {obj}
              descriptor={selectedDescriptor}
              {ctxName}
              gvr={selectedInvolvedGVR}
              namespace={obj.metadata?.namespace ?? ''}
              name={obj.metadata?.name ?? ''}
              {onrefresh}
              onopenowner={openOwnerDrawer}
            />
          {:else if selectedEvent}
            <EventDetailPanel
              event={selectedEvent}
              {now}
              onOpenInvolvedObject={openInvolvedObject}
            />
          {/if}
        {/snippet}
      </DetailDrawer>
    {/if}
  </div>
</div>
