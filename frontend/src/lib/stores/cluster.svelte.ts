import {Events} from "@wailsio/runtime";
import {
  ListContexts,
  Connect,
  Disconnect,
  Activate,
  Deactivate,
  ListNamespaces,
  CreateNamespace,
  DeleteNamespace,
  SwitchNamespace,
  GetActiveNamespace,
} from "../../../bindings/github.com/Vilsol/klados/internal/services/clusterservice.js";
import {SetReadOnly, SetLastActiveContext} from "../../../bindings/github.com/Vilsol/klados/internal/services/appservice.js";
import {StartWatch, StopWatch} from "../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js";
import {GetConfig} from "../../../bindings/github.com/Vilsol/klados/internal/services/configservice.js";
import {getLogger} from "$lib/logger";
import {preferencesStore} from "./preferences.svelte";

const log = getLogger("cluster");
// biome-ignore lint/style/noExportedImports: re-exported for consumers
import {type KubeContext, ConnectionStatus} from "../../../bindings/github.com/Vilsol/klados/internal/cluster/models.js";
import {buildKindGVRMap, resolveGVR} from "$lib/utils/relationships";
import {descriptorRegistry} from "$lib/registry";
import type {APIResource} from "../../../bindings/github.com/Vilsol/klados/internal/cluster/index.js";

export type {KubeContext};
export {ConnectionStatus};

// PermissionSet mirrors internal/cluster/permissions.go — emitted via event, not a service return type.
export interface PermissionSet {
  rules: Array<{verbs: string[]; resources: string[]; apiGroups: string[]}>;
  inferred: boolean;
}

export type ConnectionStatusType = "disconnected" | "connecting" | "connected" | "error";

const statusToString: Record<number, ConnectionStatusType> = {
  [ConnectionStatus.StatusDisconnected]: "disconnected",
  [ConnectionStatus.StatusConnecting]: "connecting",
  [ConnectionStatus.StatusConnected]: "connected",
  [ConnectionStatus.StatusError]: "error",
};

class ClusterStore {
  contexts = $state<KubeContext[]>([]);
  // activeContext = the cluster currently being viewed (set by routing components)
  activeContext = $state<string | null>(null);
  // Per-cluster namespace selection and list
  selectedNamespaces = $state<Record<string, string[]>>({});
  namespaces = $state<Record<string, string[]>>({});
  connectionStatus = $state<Record<string, ConnectionStatusType>>({});
  permissions = $state<Record<string, PermissionSet>>({});
  isReadOnly = $state<boolean>(false);
  kindGVRMap = $state<Map<string, string>>(new Map());

  private statusUnsubs: Array<() => void> = [];
  private metaUnsubs: Array<() => void> = [];
  private permUnsubs: Array<() => void> = [];
  private kubeconfigsUnsub: (() => void) | null = null;
  // Live watch on the active context's namespaces so the header dropdown
  // reflects namespaces added/removed out-of-band (kubectl, other tools).
  private nsWatch: {ctx: string; unsub: () => void; unsubResync: () => void} | null = null;

  /** Returns false when either the global read-only toggle is on or detected RBAC permits no writes. */
  canMutate(): boolean {
    if (preferencesStore.prefs.readOnly || this.isReadOnly) {
      return false;
    }
    const perms = this.permissions[this.activeContext ?? ""];
    if (!perms) {
      return true; // not yet fetched — optimistic
    }
    if (perms.inferred) {
      return true;
    }
    return perms.rules.some((r) => r.verbs.some((v) => v === "*" || v === "delete" || v === "patch" || v === "update" || v === "create"));
  }

  async setReadOnly(enabled: boolean) {
    this.isReadOnly = enabled;
    try {
      await SetReadOnly(enabled);
    } catch (e) {
      log.error("Failed to save read-only state", {error: String(e)});
    }
  }

  setDiscoveryResources(resources: APIResource[]): void {
    this.kindGVRMap = buildKindGVRMap(resources);
    descriptorRegistry.updateDiscovery(resources);
  }

  resolveOwnerGVR(apiVersion: string, kind: string): string | undefined {
    return resolveGVR(this.kindGVRMap, apiVersion, kind);
  }

  /**
   * Set the currently-viewed cluster context. Drives the monitoring lifecycle:
   * deactivates the previously-active cluster, activates the new one, and
   * persists the selection. Called by route components on mount.
   */
  async setActiveContext(ctxName: string | null) {
    const prev = this.activeContext;
    if (prev === ctxName) return;
    this.activeContext = ctxName;
    if (prev) {
      try {
        await Deactivate(prev);
      } catch (e) {
        log.warn("Deactivate failed", {ctxName: prev, error: String(e)});
      }
    }
    if (ctxName) {
      try {
        await Activate(ctxName);
      } catch (e) {
        log.warn("Activate failed", {ctxName, error: String(e)});
      }
      await this.startNamespaceWatch(ctxName);
    } else {
      this.stopNamespaceWatch();
    }
    try {
      await SetLastActiveContext(ctxName ?? "");
    } catch (e) {
      log.debug("SetLastActiveContext failed", {error: String(e)});
    }
  }

  /** Namespace list for the given context (falls back to empty array) */
  getNamespaces(ctxName: string): string[] {
    return this.namespaces[ctxName] ?? [];
  }

  /** Selected namespaces for the given context (empty = all namespaces) */
  getSelectedNamespaces(ctxName: string): string[] {
    return this.selectedNamespaces[ctxName] ?? [];
  }

  async loadContexts() {
    if (!this.kubeconfigsUnsub) {
      this.kubeconfigsUnsub = Events.On("kubeconfigs:updated", () => {
        this.loadContexts();
      });
    }
    try {
      const result = await ListContexts();
      this.contexts = result ?? [];

      for (const u of this.statusUnsubs) {
        u();
      }
      this.statusUnsubs = [];
      for (const u of this.metaUnsubs) {
        u();
      }
      this.metaUnsubs = [];
      for (const u of this.permUnsubs) {
        u();
      }
      this.permUnsubs = [];

      // Load read-only state from persisted config
      try {
        const cfg = await GetConfig();
        this.isReadOnly = cfg?.readOnly ?? false;
      } catch (e) {
        log.warn("Failed to load persisted config", {error: String(e)});
      }

      for (const ctx of this.contexts) {
        this.connectionStatus[ctx.name] = statusToString[ctx.status] ?? "disconnected";
        const unsub = Events.On(`status:${ctx.name}:connection`, (wailsEvent: unknown) => {
          const status = ((wailsEvent as {data?: unknown})?.data ?? wailsEvent) as string;
          const prev = this.connectionStatus[ctx.name];
          this.connectionStatus[ctx.name] = (status as ConnectionStatusType) ?? "disconnected";
          if (status !== "connected") return;
          if (!this.activeContext) {
            this.restoreContext(ctx.name);
          } else if (this.activeContext === ctx.name && prev !== "connected") {
            // Recovered or replaced connection: namespaces may have changed and
            // the old watch is gone — restart it unconditionally.
            this.startNamespaceWatch(ctx.name, true);
          }
        });
        this.statusUnsubs.push(unsub);
        const metaUnsub = Events.On(`metadata:${ctx.name}:cluster`, () => {
          this.loadContexts();
        });
        this.metaUnsubs.push(metaUnsub);
        const permUnsub = Events.On(`cluster:${ctx.name}:permissions`, (wailsEvent: unknown) => {
          const perms = ((wailsEvent as {data?: unknown})?.data ?? wailsEvent) as PermissionSet;
          this.permissions[ctx.name] = perms;
        });
        this.permUnsubs.push(permUnsub);
      }

      const connected = this.contexts.find((c) => (statusToString[c.status] ?? "disconnected") === "connected");
      if (connected && !this.activeContext) {
        await this.restoreContext(connected.name);
      } else if (this.activeContext) {
        // activeContext already set by routing (e.g. page refresh) — still load namespaces
        try {
          const saved = await GetActiveNamespace(this.activeContext);
          if (saved) {
            this.selectedNamespaces[this.activeContext] = [saved];
          }
        } catch (e) {
          log.debug("Could not restore saved namespace", {error: String(e)});
        }
        await this.startNamespaceWatch(this.activeContext);
      }
    } catch (e) {
      log.error("Failed to load contexts", {error: String(e)});
    }
  }

  private async restoreContext(ctxName: string) {
    await this.setActiveContext(ctxName);
    try {
      const saved = await GetActiveNamespace(ctxName);
      if (saved) {
        this.selectedNamespaces[ctxName] = [saved];
      }
    } catch (e) {
      log.debug("Could not restore saved namespace", {error: String(e)});
    }
    await this.startNamespaceWatch(ctxName);
  }

  async connect(ctxName: string) {
    this.connectionStatus[ctxName] = "connecting";
    try {
      await Connect(ctxName);
      this.connectionStatus[ctxName] = "connected";
      log.info("Cluster connected", {ctxName});
      await this.loadNamespaces(ctxName);
    } catch (e) {
      log.error("Cluster connect failed", {ctxName, error: String(e)});
      this.connectionStatus[ctxName] = "error";
    }
  }

  async disconnect(ctxName: string) {
    try {
      await Disconnect(ctxName);
      this.connectionStatus[ctxName] = "disconnected";
      log.info("Cluster disconnected", {ctxName});
      delete this.namespaces[ctxName];
      delete this.selectedNamespaces[ctxName];
      if (this.activeContext === ctxName) {
        const other = Object.entries(this.connectionStatus).find(([name, s]) => name !== ctxName && s === "connected");
        await this.setActiveContext(other ? other[0] : null);
      }
    } catch (e) {
      log.error("Failed to disconnect", {error: String(e)});
    }
  }

  async loadNamespaces(ctxName: string) {
    try {
      const result = await ListNamespaces(ctxName);
      this.namespaces[ctxName] = result ?? [];
    } catch (e) {
      log.error("Failed to load namespaces", {error: String(e)});
    }
  }

  /**
   * Load the namespace list for ctxName and keep it live via a watch on
   * core.v1.namespaces, so namespaces created/deleted outside the app update the
   * header dropdown. Idempotent for the already-watched context.
   */
  private async startNamespaceWatch(ctxName: string, force = false) {
    if (!force && this.nsWatch?.ctx === ctxName) return;
    this.stopNamespaceWatch();

    await this.loadNamespaces(ctxName);

    const eventName = `watch:${ctxName}:core.v1.namespaces:`;
    const unsub = Events.On(eventName, (wailsEvent: unknown) => {
      const event = (wailsEvent as {data?: {type: string; object?: {metadata?: {name?: string}}}})?.data;
      const name = event?.object?.metadata?.name;
      if (!event || !name) return;
      const current = this.namespaces[ctxName] ?? [];
      if (event.type === "DELETED") {
        this.namespaces[ctxName] = current.filter((n) => n !== name);
      } else if (!current.includes(name)) {
        this.namespaces[ctxName] = [...current, name].sort();
      }
    });
    // Backend emits :resync when the namespace watch hit a 410 and died —
    // reload the list and start a fresh watch or the dropdown goes stale.
    const unsubResync = Events.On(`${eventName}:resync`, () => {
      log.info("namespace watch resync", {ctxName});
      this.loadNamespaces(ctxName);
      StartWatch(ctxName, "core.v1.namespaces", "", "").catch((e) =>
        log.warn("namespace watch restart failed", {ctxName, error: String(e)}),
      );
    });
    this.nsWatch = {ctx: ctxName, unsub, unsubResync};

    // resourceVersion "" replays existing namespaces as ADDED (idempotent) and
    // then streams subsequent changes.
    StartWatch(ctxName, "core.v1.namespaces", "", "").catch((e) =>
      log.warn("namespace watch start failed", {ctxName, error: String(e)}),
    );
  }

  private stopNamespaceWatch() {
    if (!this.nsWatch) return;
    const {ctx, unsub, unsubResync} = this.nsWatch;
    this.nsWatch = null;
    unsub();
    unsubResync();
    StopWatch(ctx, "core.v1.namespaces", "").catch((e) => log.warn("namespace watch stop failed", {ctx, error: String(e)}));
  }

  async createNamespace(ctxName: string, name: string) {
    await CreateNamespace(ctxName, name);
    await this.loadNamespaces(ctxName);
  }

  async deleteNamespace(ctxName: string, name: string) {
    await DeleteNamespace(ctxName, name);
    this.namespaces[ctxName] = (this.namespaces[ctxName] ?? []).filter((n) => n !== name);
    this.selectedNamespaces[ctxName] = (this.selectedNamespaces[ctxName] ?? []).filter((n) => n !== name);
  }

  async setNamespaces(ctxName: string, namespaces: string[]) {
    this.selectedNamespaces[ctxName] = namespaces;
    const persist = namespaces.length === 1 ? namespaces[0] : "";
    try {
      await SwitchNamespace(ctxName, persist);
    } catch (e) {
      log.error("Failed to switch namespace", {error: String(e)});
    }
  }
}

export const clusterStore = new ClusterStore();
