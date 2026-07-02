import {GetDescriptors} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";
import {GetPluginDescriptors} from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
import {getLogger} from "$lib/logger";
import type {APIResource} from "$api/github.com/Vilsol/klados/internal/cluster/index.js";
import {generateDescriptor} from "./generator";

const log = getLogger("registry");
import {evaluate, parse} from "cel-js";
// biome-ignore lint/correctness/noUndeclaredDependencies: transitive dep of cel-js
import type {CstNode} from "@chevrotain/types";
import {setRegistryLoaded} from "./loaded.svelte";

export type RenderType = "text" | "badge" | "age" | "progress" | "controlledBy";

export type AlignType = "left" | "right" | "center";

export function defaultAlign(renderType: RenderType): AlignType {
  return renderType === "age" ? "right" : "left";
}

export interface ColumnDef {
  name: string;
  expr: string;
  renderType: RenderType;
  width?: number;
  align?: AlignType;
  hidden?: boolean;
}

export interface OverviewFieldDef {
  label: string;
  expr: string;
  renderType: RenderType;
}

export interface ActionDef {
  name: string;
  label: string;
  disabledWhen?: string;
  disabledReason?: string;
}

export interface DescriptorDef {
  group: string;
  version: string;
  resource: string;
  kind: string;
  gvr: string;
  columns: ColumnDef[];
  overviewFields: OverviewFieldDef[];
  detailPanels: string[];
  actions: ActionDef[];
  clusterScoped?: boolean;
}

class DescriptorRegistry {
  private descriptors = new Map<string, DescriptorDef>();
  private readonly builtins = new Map<string, DescriptorDef>();
  private availableGVRs = new Set<string>();
  private discovery = new Map<string, APIResource>();

  async load() {
    try {
      const defs = await GetDescriptors();
      this.builtins.clear();
      for (const d of defs ?? []) {
        if (!d) {
          continue;
        }
        const gKey = d.group === "" ? "core" : d.group;
        const gvr = `${gKey}.${d.version}.${d.resource}`;
        this.builtins.set(gvr, {
          group: d.group ?? "",
          version: d.version ?? "",
          resource: d.resource ?? "",
          kind: d.kind ?? "",
          gvr,
          clusterScoped: d.clusterScoped ?? false,
          columns: (d.columns ?? []).map((c) => ({
            name: c.name ?? "",
            expr: c.expr ?? "",
            renderType: ((c.renderType ?? "text") as string) as RenderType,
            width: c.width ?? undefined,
            align: (c.align ?? undefined) as AlignType | undefined,
            hidden: c.hidden ?? undefined,
          })),
          overviewFields: (d.overviewFields ?? []).map((f) => ({
            label: f.label ?? "",
            expr: f.expr ?? "",
            renderType: ((f.renderType ?? "text") as string) as RenderType,
          })),
          detailPanels: d.detailPanels ?? [],
          actions: (d.actions ?? []).map((a) => ({
            name: a.name ?? "",
            label: a.label ?? "",
            disabledWhen: a.disabledWhen ?? undefined,
            disabledReason: a.disabledReason ?? undefined,
          })),
        });
      }
      this.descriptors = new Map(this.builtins);
      await this.mergePluginDescriptors();
    } catch (e) {
      log.error("Failed to load descriptors", {error: String(e)});
    } finally {
      setRegistryLoaded();
    }
  }

  // reloadPlugins resets to builtins then re-merges from GetPluginDescriptors().
  // Call this when the plugins:loaded event fires (hot-reload).
  async reloadPlugins() {
    this.descriptors = new Map(this.builtins);
    await this.mergePluginDescriptors();
  }

  private async mergePluginDescriptors() {
    try {
      const pluginDefs = await GetPluginDescriptors();
      for (const d of pluginDefs ?? []) {
        if (!d) {
          continue;
        }
        const gKey = d.group === "" ? "core" : d.group;
        const gvr = `${gKey}.${d.version}.${d.resource}`;
        if (this.descriptors.has(gvr)) {
          const existing = this.descriptors.get(gvr) as DescriptorDef;
          const pluginColumns: ColumnDef[] = (d.columns ?? []).map((c) => ({
            name: c.name ?? "",
            expr: c.expr ?? "",
            renderType: ((c.renderType ?? "text") as string) as RenderType,
            width: c.width ?? undefined,
            align: (c.align ?? undefined) as AlignType | undefined,
            hidden: c.hidden ?? undefined,
          }));
          const addedOverview: OverviewFieldDef[] = (d.overviewFields ?? []).map((f) => ({
            label: f.label ?? "",
            expr: f.expr ?? "",
            renderType: ((f.renderType ?? "text") as string) as RenderType,
          }));
          const panelSet = new Set(existing.detailPanels);
          for (const p of d.detailPanels ?? []) {
            panelSet.add(p);
          }
          const actionNameSet = new Set(existing.actions.map((a) => a.name));
          const mergedActions: ActionDef[] = [...existing.actions];
          for (const a of d.actions ?? []) {
            const mapped: ActionDef = {name: a.name ?? "", label: a.label ?? ""};
            if (!actionNameSet.has(mapped.name)) {
              mergedActions.push(mapped);
              actionNameSet.add(mapped.name);
            }
          }
          // Create a new object — never mutate builtins references.
          // Plugin columns replace built-in columns; panels/actions are additive.
          this.descriptors.set(gvr, {
            ...existing,
            columns: pluginColumns.length > 0 ? pluginColumns : existing.columns,
            overviewFields: [...existing.overviewFields, ...addedOverview],
            detailPanels: [...panelSet],
            actions: mergedActions,
          });
        } else {
          this.descriptors.set(gvr, {
            group: d.group ?? "",
            version: d.version ?? "",
            resource: d.resource ?? "",
            kind: d.kind ?? "",
            gvr,
            columns: (d.columns ?? []).map((c) => ({
              name: c.name ?? "",
              expr: c.expr ?? "",
              renderType: ((c.renderType ?? "text") as string) as RenderType,
              width: c.width ?? undefined,
              align: (c.align ?? undefined) as AlignType | undefined,
              hidden: c.hidden ?? undefined,
            })),
            overviewFields: (d.overviewFields ?? []).map((f) => ({
              label: f.label ?? "",
              expr: f.expr ?? "",
              renderType: ((f.renderType ?? "text") as string) as RenderType,
            })),
            detailPanels: d.detailPanels ?? [],
            actions: (d.actions ?? []).map((a) => ({
              name: a.name ?? "",
              label: a.label ?? "",
              disabledWhen: a.disabledWhen ?? undefined,
              disabledReason: a.disabledReason ?? undefined,
            })),
          });
        }
      }
    } catch (e) {
      log.error("Failed to load plugin descriptors", {error: String(e)});
    }
  }

  private withControlledBy(d: DescriptorDef): DescriptorDef {
    if (d.columns.some((c) => c.name === "Controlled By")) {
      return d;
    }
    const insertBefore = d.columns.findIndex((c) => c.renderType === "age");
    const col: ColumnDef = {name: "Controlled By", expr: "metadata.ownerReferences", renderType: "controlledBy"};
    const columns = insertBefore >= 0 ? [...d.columns.slice(0, insertBefore), col, ...d.columns.slice(insertBefore)] : [...d.columns, col];
    return {...d, columns};
  }

  // Discovery reflects the active context only. On context switch this map
  // is fully replaced via `cluster.setDiscoveryResources`.
  updateDiscovery(resources: APIResource[]): void {
    this.discovery = new Map(resources.map((r) => [r.gvr, r]));
  }

  registerVirtual(gvr: string, descriptor: DescriptorDef): void {
    this.descriptors.set(gvr, descriptor);
  }

  get(gvr: string): DescriptorDef {
    const d = this.descriptors.get(gvr) ?? this.fromDiscovery(gvr) ?? this.fallback(gvr);
    return this.withControlledBy(d);
  }

  private fromDiscovery(gvr: string): DescriptorDef | undefined {
    const r = this.discovery.get(gvr);
    if (!r) return undefined;
    const d = generateDescriptor(r);
    return {
      group: d.group ?? "",
      version: d.version ?? "",
      resource: d.resource ?? "",
      kind: d.kind ?? "",
      gvr,
      clusterScoped: d.clusterScoped ?? false,
      columns: (d.columns ?? []).map((c) => ({
        name: c.name ?? "",
        expr: c.expr ?? "",
        renderType: (c.renderType ?? "text") as RenderType,
        width: c.width ?? undefined,
        align: (c.align ?? undefined) as AlignType | undefined,
        hidden: c.hidden ?? undefined,
      })),
      overviewFields: (d.overviewFields ?? []).map((f) => ({
        label: f.label ?? "",
        expr: f.expr ?? "",
        renderType: (f.renderType ?? "text") as RenderType,
      })),
      detailPanels: d.detailPanels ?? [],
      actions: (d.actions ?? []).map((a) => ({
        name: a.name ?? "",
        label: a.label ?? "",
        disabledWhen: a.disabledWhen ?? undefined,
        disabledReason: a.disabledReason ?? undefined,
      })),
    };
  }

  private fallback(gvr: string): DescriptorDef {
    const parts = gvr.split(".");
    const resource = parts.at(-1) ?? gvr;
    const version = parts.at(-2) ?? "";
    const group = parts.slice(0, -2).join(".");
    return {
      group: group === "core" ? "" : group,
      version,
      resource,
      kind: "",
      gvr,
      columns: [
        {name: "Name", expr: "metadata.name", renderType: "text"},
        {name: "Namespace", expr: "metadata.namespace", renderType: "text"},
        {name: "Age", expr: "metadata.creationTimestamp", renderType: "age"},
      ],
      overviewFields: [
        {label: "Namespace", expr: "metadata.namespace", renderType: "text"},
        {label: "Age", expr: "metadata.creationTimestamp", renderType: "age"},
      ],
      detailPanels: ["overview", "yaml"],
      actions: [{name: "delete", label: "Delete"}],
    };
  }

  list(): DescriptorDef[] {
    return Array.from(this.descriptors.values());
  }

  listDiscoveryGVRs(): APIResource[] {
    const out: APIResource[] = [];
    for (const [gvr, r] of this.discovery) {
      if (this.descriptors.has(gvr)) continue;
      if (this.builtins.has(gvr)) continue;
      out.push(r);
    }
    out.sort((a, b) => {
      const kindCmp = a.kind.localeCompare(b.kind);
      if (kindCmp !== 0) return kindCmp;
      return a.gvr.localeCompare(b.gvr);
    });
    return out;
  }

  /** Called by Sidebar after each discovery event to update the set of available GVRs. */
  setAvailableGVRs(gvrs: string[]): void {
    this.availableGVRs = new Set(gvrs);
  }

  /**
   * Returns true if the GVR is available on the connected cluster.
   * Before the first discovery event (empty set) returns true to avoid
   * briefly flashing the entire sidebar as disabled on connect.
   */
  isGVRAvailable(gvr: string): boolean {
    if (this.availableGVRs.size === 0) {
      return true;
    }
    return this.availableGVRs.has(gvr);
  }
}

export const descriptorRegistry = new DescriptorRegistry();

// Simple dot-path: letters, digits, underscores, separated by dots — no brackets, operators, or calls
const simplePath = /^[a-zA-Z_]\w*(\.[a-zA-Z_]\w*)*$/;

// Cache: expr → split path segments (for simple paths) or parsed CST (for CEL)
const exprCache = new Map<string, string[] | CstNode>();

function resolveExpr(expr: string): string[] | CstNode {
  let cached = exprCache.get(expr);
  if (cached) {
    return cached;
  }
  if (simplePath.test(expr)) {
    cached = expr.split(".");
  } else {
    const result = parse(expr);
    cached = result.isSuccess ? result.cst : expr.split(".");
  }
  exprCache.set(expr, cached);
  return cached;
}

export function evalExpr(expr: string, obj: Record<string, unknown>): unknown {
  const resolved = resolveExpr(expr);
  if (Array.isArray(resolved)) {
    let cur: unknown = obj;
    for (const p of resolved) {
      if (cur == null) {
        return "";
      }
      cur = (cur as Record<string, unknown>)[p];
    }
    return cur ?? "";
  }
  // Pre-parsed CEL CST
  const ctx = {
    metadata: obj.metadata ?? {},
    spec: obj.spec ?? {},
    status: obj.status ?? {},
    type: obj.type ?? "",
  };
  try {
    return evaluate(resolved, ctx) ?? "";
  } catch {
    return "";
  }
}
