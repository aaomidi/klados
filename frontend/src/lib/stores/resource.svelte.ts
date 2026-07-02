import {Events} from "@wailsio/runtime";
import {ListResourcesWithVersion, StartWatch, StopWatch} from "../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js";
import {getLogger} from "$lib/logger";
import type {KubernetesResource} from "$lib/types";
import {resourceCache} from "./resourceCache.svelte";

const log = getLogger("resource");

interface WatchEvent {
  type: "ADDED" | "MODIFIED" | "DELETED";
  object: KubernetesResource;
}

function resourceKey(obj: KubernetesResource): string {
  return `${obj.metadata?.namespace ?? ""}/${obj.metadata?.name ?? ""}`;
}

export class ResourceStore {
  items = $state<KubernetesResource[]>([]);
  loading = $state(false);
  error = $state<string | null>(null);
  lastLoadMs = $state<number | null>(null);

  private contextName = "";
  private gvr = "";
  private namespace = "";
  private eventName = "";
  private resyncEventName = "";
  private unsub: (() => void) | null = null;
  private unsubResync: (() => void) | null = null;
  private unsubStatus: (() => void) | null = null;
  private lastStatus = "";
  private generation = 0;

  async start(contextName: string, gvr: string, namespace: string) {
    this.stop();
    const gen = this.generation; // captured after stop() bumped it

    this.contextName = contextName;
    this.gvr = gvr;
    this.namespace = namespace;
    this.eventName = `watch:${contextName}:${gvr}:${namespace}`;
    this.resyncEventName = `${this.eventName}:resync`;
    this.loading = true;
    this.error = null;
    this.lastLoadMs = null;

    // Events.On returns an unsubscribe fn; callback receives WailsEvent { name, data }
    this.unsub = Events.On(this.eventName, (wailsEvent: unknown) => {
      this.handleEvent((wailsEvent as {data: WatchEvent}).data);
    });
    this.unsubResync = Events.On(this.resyncEventName, () => {
      this.resync(contextName, gvr, namespace, gen).catch((e) =>
        log.warn("resync failed", {contextName, gvr, namespace, error: String(e)}),
      );
    });

    // Re-list when the backend connection recovers: watches may have died or
    // been replaced (kubeconfig re-import) while this page was open.
    this.lastStatus = "";
    this.unsubStatus = Events.On(`status:${contextName}:connection`, (wailsEvent: unknown) => {
      const status = ((wailsEvent as {data?: unknown})?.data ?? wailsEvent) as string;
      const prev = this.lastStatus;
      this.lastStatus = status;
      // prev "" = no status seen since start() — the page-load connected event.
      // Any other non-connected prev (error, disconnected, connecting) means the
      // connection dropped or was replaced, so re-list.
      if (status === "connected" && prev !== "" && prev !== "connected") {
        log.info("connection recovered — re-listing", {contextName, gvr});
        this.loadAndWatch(contextName, gvr, namespace, gen, performance.now()).catch((e) =>
          log.warn("reconnect re-list failed", {contextName, gvr, error: String(e)}),
        );
      }
    });

    await this.loadAndWatch(contextName, gvr, namespace, gen, performance.now());
  }

  private async loadAndWatch(
    contextName: string,
    gvr: string,
    namespace: string,
    gen: number,
    t0: number,
  ) {
    try {
      const tList = performance.now();
      const result = await ListResourcesWithVersion(contextName, gvr, namespace);
      if (gen !== this.generation) {
        return;
      }
      const listMs = performance.now() - tList;

      const items = result?.items ?? [];
      const rv = result?.resourceVersion ?? "";

      const map = new Map<string, KubernetesResource>();
      for (const obj of items) {
        map.set(resourceKey(obj), obj as KubernetesResource);
        resourceCache.upsert(contextName, gvr, obj as Record<string, unknown>);
      }
      this.items = Array.from(map.values());
      this.loading = false;
      this.lastLoadMs = Math.round(listMs);
      const count = this.items.length;

      requestAnimationFrame(() => {
        if (gen !== this.generation) {
          return;
        }
        requestAnimationFrame(() => {
          if (gen !== this.generation) {
            return;
          }
          const total = Math.round(performance.now() - t0);
          log.debug("perf", {gvr, count, listMs: Math.round(listMs), interactiveMs: total});
        });
      });

      // Start watch in background — event listener is already subscribed.
      // Pass the list's resourceVersion so the server doesn't replay every existing
      // object as a synthetic ADDED event.
      StartWatch(contextName, gvr, namespace, rv).catch((e) => log.warn("StartWatch failed", {contextName, gvr, namespace, error: String(e)}));
    } catch (e) {
      if (gen !== this.generation) {
        return;
      }
      this.error = (e instanceof Error ? e.message : null) ?? String(e);
      this.loading = false;
    }
  }

  // Called when the backend signals that the watch hit a 410 Gone and needs a fresh list.
  private async resync(contextName: string, gvr: string, namespace: string, gen: number) {
    if (gen !== this.generation) return;
    log.info("watch resync", {contextName, gvr, namespace});
    await this.loadAndWatch(contextName, gvr, namespace, gen, performance.now());
  }

  stop() {
    this.generation++;
    if (this.unsub) {
      this.unsub();
      this.unsub = null;
    }
    if (this.unsubResync) {
      this.unsubResync();
      this.unsubResync = null;
    }
    if (this.unsubStatus) {
      this.unsubStatus();
      this.unsubStatus = null;
    }
    this.lastStatus = "";
    if (this.contextName && this.gvr) {
      StopWatch(this.contextName, this.gvr, this.namespace).catch((e) => log.warn("StopWatch failed", {error: String(e)}));
    }
    this.items = [];
    this.loading = false;
    this.error = null;
  }

  private handleEvent(event: WatchEvent) {
    if (!event?.object) {
      return;
    }
    const obj = event.object;
    const key = resourceKey(obj);

    if (event.type === "DELETED") {
      const uid = obj.metadata?.uid;
      if (uid) resourceCache.remove(this.contextName, this.gvr, uid);
      this.items = this.items.filter((i) => resourceKey(i) !== key);
    } else {
      resourceCache.upsert(this.contextName, this.gvr, obj as Record<string, unknown>);
      const idx = this.items.findIndex((i) => resourceKey(i) === key);
      if (idx >= 0) {
        const next = [...this.items];
        next[idx] = obj;
        this.items = next;
      } else {
        this.items = [...this.items, obj];
      }
    }
  }
}

export function createResourceStore() {
  return new ResourceStore();
}
