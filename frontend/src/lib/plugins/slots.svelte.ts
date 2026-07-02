import {Events} from "@wailsio/runtime";
import {mount} from "svelte";
import {
  GetPluginDetailTabs,
  GetPluginCommands,
  GetPluginOverviewFields,
  GetPluginListColumns,
  GetPluginContextMenuItems,
  GetPluginHeaderWidgets,
  GetPluginStatusBarWidgets,
  InvokeCommand,
} from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
import {getLogger} from "$lib/logger";

const log = getLogger("plugins");
// biome-ignore lint/style/noExportedImports: re-exported for consumers
import type {PermsSummary} from "$api/github.com/Vilsol/klados/internal/plugin/models.js";
import {notificationStore} from "$lib/stores/notification.svelte.js";
import {streamingStore} from "$lib/stores/streaming.svelte.js";
import {clusterStore} from "$lib/stores/cluster.svelte.js";
import {createPluginContext} from "$lib/plugins/context.js";
import type {PluginManifest} from "$lib/plugins/types/manifest.js";
import {ListResources, GetResource} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";

export type {PermsSummary};

export interface ResourcePerm {
  group: string;
  version: string;
  resource: string;
  verbs: string[];
}

export interface RegisteredDetailTab {
  pluginName: string;
  gvr: string;
  id: string;
  label: string;
  component: string;
  perms: PermsSummary;
}

export interface RegisteredCommand {
  pluginName: string;
  id: string;
  label: string;
  icon?: string;
  component?: string;
  perms: PermsSummary;
  action: () => void;
}

export interface RegisteredOverviewField {
  pluginName: string;
  gvr: string;
  id: string;
  label: string;
  component: string;
}

export interface RegisteredListColumn {
  pluginName: string;
  gvr: string;
  id: string;
  label: string;
  component: string;
}

export interface RegisteredContextMenuItem {
  pluginName: string;
  gvr: string;
  id: string;
  label: string;
  component: string;
}

export interface RegisteredHeaderWidget {
  pluginName: string;
  id: string;
  component: string;
}

export interface RegisteredStatusBarWidget {
  pluginName: string;
  id: string;
  component: string;
}

class SlotRegistry {
  detailTabs = $state<RegisteredDetailTab[]>([]);
  commands = $state<RegisteredCommand[]>([]);
  overviewFields = $state<RegisteredOverviewField[]>([]);
  listColumns = $state<RegisteredListColumn[]>([]);
  contextMenuItems = $state<RegisteredContextMenuItem[]>([]);
  headerWidgets = $state<RegisteredHeaderWidget[]>([]);
  statusBarWidgets = $state<RegisteredStatusBarWidget[]>([]);

  getDetailTabs(gvr: string): RegisteredDetailTab[] {
    return this.detailTabs.filter((t) => t.gvr === gvr);
  }

  getCommands(): RegisteredCommand[] {
    return this.commands;
  }

  getOverviewFields(gvr: string): RegisteredOverviewField[] {
    return this.overviewFields.filter((f) => f.gvr === gvr);
  }

  getListColumns(gvr: string): RegisteredListColumn[] {
    return this.listColumns.filter((c) => c.gvr === gvr);
  }

  getContextMenuItems(gvr: string): RegisteredContextMenuItem[] {
    return this.contextMenuItems.filter((c) => c.gvr === gvr);
  }

  getHeaderWidgets(): RegisteredHeaderWidget[] {
    return this.headerWidgets;
  }

  getStatusBarWidgets(): RegisteredStatusBarWidget[] {
    return this.statusBarWidgets;
  }

  registerCommand(cmd: RegisteredCommand): void {
    this.commands = [...this.commands, cmd];
  }

  unregisterPlugin(pluginName: string): void {
    this.detailTabs = this.detailTabs.filter((t) => t.pluginName !== pluginName);
    this.commands = this.commands.filter((c) => c.pluginName !== pluginName);
    this.overviewFields = this.overviewFields.filter((f) => f.pluginName !== pluginName);
    this.listColumns = this.listColumns.filter((c) => c.pluginName !== pluginName);
    this.contextMenuItems = this.contextMenuItems.filter((c) => c.pluginName !== pluginName);
    this.headerWidgets = this.headerWidgets.filter((w) => w.pluginName !== pluginName);
    this.statusBarWidgets = this.statusBarWidgets.filter((w) => w.pluginName !== pluginName);
  }

  async initFromBackend(): Promise<void> {
    try {
      const [tabs, cmds, overviewFields, listColumns, contextMenuItems, headerWidgets, statusBarWidgets] = await Promise.all([
        GetPluginDetailTabs(),
        GetPluginCommands(),
        GetPluginOverviewFields(""),
        GetPluginListColumns(""),
        GetPluginContextMenuItems(""),
        GetPluginHeaderWidgets(),
        GetPluginStatusBarWidgets(),
      ]);
      this.detailTabs = (tabs ?? []).map((t) => ({
        pluginName: t.pluginName ?? "",
        gvr: t.gvr ?? "",
        id: t.id ?? "",
        label: t.label ?? "",
        component: t.component ?? "",
        perms: t.perms ?? {},
      }));
      this.commands = (cmds ?? []).map((c) => ({
        pluginName: c.pluginName ?? "",
        id: c.id ?? "",
        label: c.label ?? "",
        icon: c.icon ?? undefined,
        component: c.component ?? undefined,
        perms: c.perms ?? {},
        action: c.component
          ? () => {
              invokeComponentCommand(c.pluginName ?? "", c.component as string, c.perms ?? {}, getBasePluginURL()).catch((e) =>
                notificationStore.error(`Plugin "${c.pluginName}" failed`, String(e)),
              );
            }
          : () => {
              const pluginName = c.pluginName ?? "";
              const id = c.id ?? "";
              log.info("Calling InvokeCommand", {pluginName, id});
              InvokeCommand(pluginName, id)
                .then(() => log.info("InvokeCommand resolved ok"))
                .catch((e) => {
                  log.error("InvokeCommand rejected", {error: String(e)});
                  notificationStore.error(`Plugin "${pluginName}" failed`, String(e));
                });
            },
      }));
      this.overviewFields = (overviewFields ?? []).map((f) => ({
        pluginName: f.pluginName ?? "",
        gvr: f.gvr ?? "",
        id: f.id ?? "",
        label: f.label ?? "",
        component: f.component ?? "",
      }));
      this.listColumns = (listColumns ?? []).map((c) => ({
        pluginName: c.pluginName ?? "",
        gvr: c.gvr ?? "",
        id: c.id ?? "",
        label: c.label ?? "",
        component: c.component ?? "",
      }));
      this.contextMenuItems = (contextMenuItems ?? []).map((c) => ({
        pluginName: c.pluginName ?? "",
        gvr: c.gvr ?? "",
        id: c.id ?? "",
        label: c.label ?? "",
        component: c.component ?? "",
      }));
      this.headerWidgets = (headerWidgets ?? []).map((w) => ({
        pluginName: w.pluginName ?? "",
        id: w.id ?? "",
        component: w.component ?? "",
      }));
      this.statusBarWidgets = (statusBarWidgets ?? []).map((w) => ({
        pluginName: w.pluginName ?? "",
        id: w.id ?? "",
        component: w.component ?? "",
      }));
    } catch {
      // No plugins loaded or backend unavailable — non-fatal
    }
  }
}

export const slotRegistry = new SlotRegistry();

function getBasePluginURL(): string | null {
  return streamingStore.config ? streamingStore.pluginsBaseUrl() : null;
}

async function invokeComponentCommand(pluginName: string, component: string, perms: PermsSummary, basePluginURL: string | null) {
  if (!basePluginURL) {
    return;
  }
  const url = `${basePluginURL}/${pluginName}/${component}`;
  const mod = await import(/* @vite-ignore */ url);
  if (!mod?.default) {
    return;
  }
  const ctx = buildCommandContext(pluginName, perms);
  mount(mod.default, {target: document.body, props: {ctx}});
}

function buildCommandContext(pluginName: string, perms: PermsSummary) {
  const manifest = {
    schemaVersion: 1 as const,
    name: pluginName,
    version: "",
    displayName: "",
    minHostVersion: "",
    permissions: {
      resources: perms.resources?.map((p) => ({...p, verbs: p.verbs as string[]})),
      logs: perms.logs || undefined,
      exec: perms.exec || undefined,
      storage: perms.storage || undefined,
      events: perms.events || undefined,
    },
  };
  const ctx = clusterStore.activeContext ?? "";
  return createPluginContext(manifest as unknown as PluginManifest, {
    clusterName: ctx,
    clusterVersion: "",
    namespace: clusterStore.getSelectedNamespaces(ctx)[0] ?? "",
    listResources: (g, n) => ListResources(ctx, g, n ?? ""),
    getResource: (g, n, name) => GetResource(ctx, g, n, name),
  });
}

if (typeof window !== "undefined") {
  Events.On("plugins:loaded", () => {
    slotRegistry.initFromBackend();
  });

  Events.On("plugin:reloading", (wailsEvent: unknown) => {
    const ev = wailsEvent as {data?: {name?: string}; name?: string} | undefined;
    const name = ev?.data?.name ?? ev?.name;
    if (name) {
      slotRegistry.unregisterPlugin(name);
      notificationStore.push(`Reloading plugin "${name}"...`, "info");
    }
  });

  Events.On("plugin:loaded", (wailsEvent: unknown) => {
    const ev = wailsEvent as {data?: {name?: string}; name?: string} | undefined;
    const name = ev?.data?.name ?? ev?.name;
    slotRegistry.initFromBackend();
    if (name) {
      notificationStore.push(`Plugin "${name}" reloaded`, "success");
    }
  });

  Events.On("plugin:error", (wailsEvent: unknown) => {
    const ev = wailsEvent as {data?: {name?: string; error?: string}} | undefined;
    const data = ev?.data;
    const name = data?.name;
    const error = data?.error;
    if (name) {
      notificationStore.error(`Plugin "${name}" failed`, error);
    }
  });

  slotRegistry.initFromBackend();
}
