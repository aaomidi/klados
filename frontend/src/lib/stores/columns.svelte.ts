import {
  GetColumnPrefs,
  GetCompactRows,
  DeleteColumnPrefs,
  SetColumnPrefs,
  SetCompactRows,
} from "$api/github.com/Vilsol/klados/internal/services/configservice.js";
import {GVRColumnPrefs, ColumnSettings, SortPrefs} from "$api/github.com/Vilsol/klados/internal/config/models.js";
import {descriptorRegistry} from "../registry/index.js";
import type {ColumnDef} from "../registry/index.js";

class ColumnStore {
  visibleColumns = $state<ColumnDef[]>([]);
  allColumns = $state<{col: ColumnDef; visible: boolean}[]>([]);
  sortState = $state<{column: string; direction: "asc" | "desc"} | null>(null);
  compact = $state<boolean>(false);
  #pinnedSet = $state<Set<string>>(new Set());

  #gvr = "";
  #saveTimer: ReturnType<typeof setTimeout> | null = null;

  async loadForGVR(gvr: string): Promise<void> {
    if (this.#saveTimer !== null) {
      clearTimeout(this.#saveTimer);
      this.#saveTimer = null;
    }
    this.#gvr = gvr;

    const [prefs, compact] = await Promise.all([GetColumnPrefs(gvr), GetCompactRows()]);
    this.compact = compact;
    this.#applyPrefs(prefs);
  }

  #applyPrefs(prefs: GVRColumnPrefs | null): void {
    const descriptor = descriptorRegistry.get(this.#gvr);
    const pool = descriptor.columns;

    // Build name → ColumnDef map with width overrides applied
    const poolMap = new Map<string, ColumnDef>(pool.map((c) => [c.name, {...c}]));
    if (prefs?.columns) {
      for (const [name, settings] of Object.entries(prefs.columns)) {
        if (settings?.width !== undefined && poolMap.has(name)) {
          const existing = poolMap.get(name);
          if (existing) {
            poolMap.set(name, {...existing, width: settings.width});
          }
        }
      }
    }

    // Determine visible order
    let visibleNames: string[];
    if (prefs?.order && prefs.order.length > 0) {
      visibleNames = prefs.order.filter((name) => poolMap.has(name));
    } else {
      visibleNames = pool.filter((c) => !c.hidden).map((c) => c.name);
    }

    const visibleSet = new Set(visibleNames);

    // Pinned set: explicit prefs OR default to ["Name"] when no prefs saved at all
    const explicitPinned = prefs?.pinned ?? [];
    const pinned = explicitPinned.length > 0
      ? explicitPinned.filter((n) => visibleSet.has(n))
      : (prefs === null && visibleSet.has("Name") ? ["Name"] : []);
    this.#pinnedSet = new Set(pinned);

    // Reorder visibleNames so pinned columns are first (preserving pinned order)
    const pinnedFirst = [...pinned, ...visibleNames.filter((n) => !this.#pinnedSet.has(n))];
    this.visibleColumns = pinnedFirst.map((name) => poolMap.get(name)).filter((c): c is ColumnDef => c !== undefined);

    this.allColumns = pool.map((c) => ({
      col: poolMap.get(c.name) as ColumnDef,
      visible: visibleSet.has(c.name),
    }));

    if (prefs?.sort) {
      this.sortState = {column: prefs.sort.column, direction: prefs.sort.direction as "asc" | "desc"};
    } else {
      // Default: sort by Age descending (newest first) when the column exists.
      const ageCol = this.visibleColumns.find((c) => c.renderType === "age");
      this.sortState = ageCol ? {column: ageCol.name, direction: "desc"} : null;
    }
  }

  setColumnVisible(name: string, visible: boolean): void {
    if (name === "Name") {
      return;
    }
    if (!visible && this.#pinnedSet.has(name)) {
      return; // unpin first
    }

    const entry = this.allColumns.find((e) => e.col.name === name);
    if (!entry || entry.visible === visible) {
      return;
    }

    const col = entry.col;
    this.allColumns = this.allColumns.map((e) => (e.col.name === name ? {...e, visible} : e));
    if (visible) {
      this.visibleColumns = [...this.visibleColumns, col];
    } else {
      this.visibleColumns = this.visibleColumns.filter((c) => c.name !== name);
    }
    this.#save();
  }

  moveColumn(name: string, direction: "up" | "down"): void {
    const idx = this.visibleColumns.findIndex((c) => c.name === name);
    if (idx === -1) {
      return;
    }
    if (direction === "up" && idx === 0) {
      return;
    }
    if (direction === "down" && idx === this.visibleColumns.length - 1) {
      return;
    }

    const next = [...this.visibleColumns];
    const swapIdx = direction === "up" ? idx - 1 : idx + 1;
    [next[idx], next[swapIdx]] = [next[swapIdx], next[idx]];
    this.visibleColumns = next;
    this.#save();
  }

  resizeColumn(name: string, width: number): void {
    this.#setWidth(name, width);
    this.#debouncedSave();
  }

  autoFitColumn(name: string, width: number): void {
    this.#setWidth(name, width);
    this.#debouncedSave();
  }

  #setWidth(name: string, width: number): void {
    this.visibleColumns = this.visibleColumns.map((c) => (c.name === name ? {...c, width} : c));
    this.allColumns = this.allColumns.map((e) => (e.col.name === name ? {...e, col: {...e.col, width}} : e));
  }

  setSort(column: string, direction: "asc" | "desc"): void {
    this.sortState = {column, direction};
    this.#save();
  }

  reset(): void {
    if (this.#saveTimer !== null) {
      clearTimeout(this.#saveTimer);
      this.#saveTimer = null;
    }
    DeleteColumnPrefs(this.#gvr);
    this.#applyPrefs(null);
  }

  async setCompact(value: boolean): Promise<void> {
    this.compact = value;
    await SetCompactRows(value);
  }

  #buildPrefs(): GVRColumnPrefs {
    return new GVRColumnPrefs({
      order: this.visibleColumns.map((c) => c.name),
      columns: Object.fromEntries(
        this.allColumns.filter(({col}) => col.width !== undefined).map(({col}) => [col.name, new ColumnSettings({width: col.width})]),
      ),
      sort: this.sortState ? new SortPrefs({column: this.sortState.column, direction: this.sortState.direction}) : null,
      pinned: [...this.#pinnedSet],
    });
  }

  #save(): void {
    if (this.#saveTimer !== null) {
      clearTimeout(this.#saveTimer);
      this.#saveTimer = null;
    }
    SetColumnPrefs(this.#gvr, this.#buildPrefs());
  }

  #debouncedSave(): void {
    if (this.#saveTimer !== null) {
      clearTimeout(this.#saveTimer);
    }
    this.#saveTimer = setTimeout(() => {
      this.#saveTimer = null;
      SetColumnPrefs(this.#gvr, this.#buildPrefs());
    }, 300);
  }

  reorderVisible(names: string[]): void {
    const currentSet = new Set(this.visibleColumns.map((c) => c.name));
    const namedAndVisible = names.filter((n) => currentSet.has(n));
    const pinned = [...this.#pinnedSet].filter((n) => currentSet.has(n));
    const namedRest = namedAndVisible.filter((n) => !this.#pinnedSet.has(n));
    // Append any visible-but-not-named non-pinned columns at the end (preserving their relative order)
    const namedSet = new Set(namedAndVisible);
    const trailing = this.visibleColumns
      .map((c) => c.name)
      .filter((n) => !namedSet.has(n) && !this.#pinnedSet.has(n));
    const finalOrder = [...pinned, ...namedRest, ...trailing];
    const byName = new Map(this.visibleColumns.map((c) => [c.name, c]));
    this.visibleColumns = finalOrder.map((n) => byName.get(n)).filter((c): c is ColumnDef => c !== undefined);
    this.#save();
  }

  setPinned(name: string, pinned: boolean): void {
    if (!this.visibleColumns.some((c) => c.name === name)) {
      return;
    }
    const next = new Set(this.#pinnedSet);
    if (pinned) {
      next.add(name);
    } else {
      next.delete(name);
    }
    this.#pinnedSet = next;

    const without = this.visibleColumns.filter((c) => c.name !== name);
    const col = this.visibleColumns.find((c) => c.name === name);
    if (col) {
      const pinnedCount = without.filter((c) => this.#pinnedSet.has(c.name)).length;
      this.visibleColumns = [...without.slice(0, pinnedCount), col, ...without.slice(pinnedCount)];
    }
    this.#save();
  }

  isPinned(name: string): boolean {
    return this.#pinnedSet.has(name);
  }

  pinnedNames(): string[] {
    return this.visibleColumns.filter((c) => this.#pinnedSet.has(c.name)).map((c) => c.name);
  }
}

export const columnStore = new ColumnStore();
