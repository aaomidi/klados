import {preferencesStore, type VolumeBrowserConfig} from "./preferences.svelte";
import {notificationStore} from "./notification.svelte";
import {bottomPanelStore} from "./bottom-panel.svelte";
import {
  Spawn,
  Replace,
  AttachTab,
} from "$api/github.com/Vilsol/klados/internal/services/volumebrowserservice.js";
import type {
  SpawnOverridesDTO,
  SpawnRequestDTO,
  SpawnResult,
} from "$api/github.com/Vilsol/klados/internal/services/models.js";
import {getLogger} from "$lib/logger";

const log = getLogger("volume-browser");

export interface DialogState {
  namespace: string;
  pvcName: string;
  initial: VolumeBrowserConfig;
  resolve: (overrides: SpawnOverridesDTO | null) => void;
}

export interface CollisionState {
  existingPodName: string;
  existingId: string;
  pvcName: string;
  resolve: (choice: "attach" | "replace" | "cancel") => void;
}

export interface SpawnOptions {
  forceDialog?: boolean;
  shiftHeld?: boolean;
}

function isCollisionError(e: unknown): e is {existingPodName: string; existingId: string; error?: string} {
  return (
    typeof e === "object" &&
    e !== null &&
    "existingPodName" in e &&
    typeof (e as {existingPodName: unknown}).existingPodName === "string" &&
    (e as {existingPodName: string}).existingPodName !== ""
  );
}

function errorMessage(e: unknown): string {
  if (typeof e === "object" && e !== null) {
    const maybe = e as {message?: string; error?: string};
    return maybe.message ?? maybe.error ?? String(e);
  }
  return String(e);
}

function overridesFromPrefs(cfg: VolumeBrowserConfig): SpawnOverridesDTO {
  const o: Partial<SpawnOverridesDTO> = {};
  if (cfg.image) o.image = cfg.image;
  if (cfg.mountPath) o.mountPath = cfg.mountPath;
  if (cfg.readOnly != null) o.readOnly = cfg.readOnly;
  if (cfg.activeDeadlineSeconds != null) o.activeDeadlineSeconds = cfg.activeDeadlineSeconds;
  if (cfg.resources != null) {
    o.resources = cfg.resources as SpawnOverridesDTO["resources"];
  }
  if (cfg.nodeSelector && Object.keys(cfg.nodeSelector).length > 0) {
    o.nodeSelector = cfg.nodeSelector;
  }
  if (cfg.tolerations && cfg.tolerations.length > 0) {
    o.tolerations = cfg.tolerations;
  }
  return o as SpawnOverridesDTO;
}

class VolumeBrowserStore {
  dialog = $state<DialogState | null>(null);
  collision = $state<CollisionState | null>(null);
  // Stash of last-used spawn requests, keyed by managed id. Consumed by
  // retry() when the user hits "Delete & Retry" on a stuck pending terminal.
  lastRequests = new Map<string, SpawnRequestDTO>();

  /**
   * Re-spawn the managed pod identified by `managedId` using the same request
   * used to originally spawn it. Called by the terminal-pending panel's
   * "Delete & Retry" action. No-op if the original request isn't known.
   */
  async retry(managedId: string): Promise<void> {
    const req = this.lastRequests.get(managedId);
    if (!req) {
      notificationStore.error("Cannot retry volume browser", "Original request not found");
      return;
    }
    const oldTab = bottomPanelStore.tabs.find((t) => t.managedId === managedId);
    const oldTabId = oldTab?.id;
    try {
      const result = (await Replace(managedId, req)) as SpawnResult;
      this.lastRequests.delete(managedId);
      this.lastRequests.set(result.id, req);
      if (oldTabId) {
        // Server-side Replace already tore down the old pod — don't double-Stop.
        bottomPanelStore.closeTab(oldTabId, {skipStop: true});
      }
      await this.afterSpawn(req.contextName, result);
    } catch (e: unknown) {
      notificationStore.error("Failed to retry volume browser", errorMessage(e));
      log.error("volume-browser retry failed", {error: errorMessage(e)});
    }
  }

  async spawn(ctxName: string, namespace: string, pvcName: string, opts: SpawnOptions = {}): Promise<void> {
    const prefs = preferencesStore.prefs.volumeBrowser;
    const promptBefore = prefs.promptBeforeSpawn === true;
    const shouldPrompt = opts.forceDialog || opts.shiftHeld || promptBefore;

    let overrides: SpawnOverridesDTO | undefined;
    if (shouldPrompt) {
      const result = await this.promptDialog(namespace, pvcName, prefs);
      if (result == null) {
        return; // cancelled
      }
      overrides = result;
    } else {
      overrides = overridesFromPrefs(prefs);
    }

    await this.performSpawn(ctxName, namespace, pvcName, overrides);
  }

  private promptDialog(
    namespace: string,
    pvcName: string,
    initial: VolumeBrowserConfig,
  ): Promise<SpawnOverridesDTO | null> {
    return new Promise((resolve) => {
      this.dialog = {
        namespace,
        pvcName,
        initial: {...initial},
        resolve: (v) => {
          this.dialog = null;
          resolve(v);
        },
      };
    });
  }

  private promptCollision(existingPodName: string, existingId: string, pvcName: string): Promise<"attach" | "replace" | "cancel"> {
    return new Promise((resolve) => {
      this.collision = {
        existingPodName,
        existingId,
        pvcName,
        resolve: (v) => {
          this.collision = null;
          resolve(v);
        },
      };
    });
  }

  private async performSpawn(
    ctxName: string,
    namespace: string,
    pvcName: string,
    overrides: SpawnOverridesDTO | undefined,
    replaceId?: string,
  ): Promise<void> {
    const req: SpawnRequestDTO = {
      contextName: ctxName,
      namespace,
      pvcName,
      ...(overrides && Object.keys(overrides).length > 0 ? {overrides} : {}),
    } as SpawnRequestDTO;

    let result: SpawnResult;
    try {
      if (replaceId) {
        result = (await Replace(replaceId, req)) as SpawnResult;
      } else {
        result = (await Spawn(req)) as SpawnResult;
      }
    } catch (e: unknown) {
      if (replaceId) {
        notificationStore.error("Failed to replace volume browser", errorMessage(e));
        log.error("volume-browser replace failed", {error: errorMessage(e)});
        return;
      }
      if (isCollisionError(e)) {
        const choice = await this.promptCollision(e.existingPodName, e.existingId, pvcName);
        if (choice === "cancel") {
          return;
        }
        if (choice === "attach") {
          result = {id: e.existingId, namespace, podName: e.existingPodName} as SpawnResult;
        } else {
          await this.performSpawn(ctxName, namespace, pvcName, overrides, e.existingId);
          return;
        }
      } else {
        notificationStore.error("Failed to spawn volume browser", errorMessage(e));
        log.error("volume-browser spawn failed", {error: errorMessage(e)});
        return;
      }
    }

    this.lastRequests.set(result.id, req);
    await this.afterSpawn(ctxName, result);
  }

  private async afterSpawn(ctxName: string, result: SpawnResult): Promise<void> {
    // No navigation — the bottom-panel terminal-pending tab is the primary UX.
    // A global drawer-open primitive would be needed for drawer-style opening,
    // and isn't worth the complexity for this feature.
    const tabId = bottomPanelStore.addTab({
      kind: "terminal-pending",
      ctxName,
      gvr: "core.v1.pods",
      namespace: result.namespace,
      name: result.podName,
      resourceKind: "Pod",
      resourceName: result.podName,
      obj: {},
      managedId: result.id,
    });

    try {
      await AttachTab(result.id, tabId);
    } catch (e) {
      log.warn("AttachTab failed", {error: errorMessage(e)});
    }
  }
}

export const volumeBrowserStore = new VolumeBrowserStore();
