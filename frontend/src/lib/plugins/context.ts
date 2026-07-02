import {Events} from "@wailsio/runtime";
import type {PluginManifest} from "./types/manifest.js";
import type {PluginContext} from "./types/context.js";
import {assertGVRPermission} from "./permissions.js";
import {StartLogStream, StopLogStream} from "$api/github.com/Vilsol/klados/internal/services/logservice.js";
import {OpenExecSession, CloseExecSession} from "$api/github.com/Vilsol/klados/internal/services/execservice.js";
import {
  GetPluginStorageKey,
  SetPluginStorageKey,
  DeletePluginStorageKey,
} from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
import {StartWatch, StopWatch} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";

export interface HostServices {
  clusterName: string;
  clusterVersion: string;
  namespace: string;
  listResources(gvr: string, ns?: string): Promise<unknown[]>;
  getResource(gvr: string, ns: string, name: string): Promise<unknown>;
}

export function createPluginContext(manifest: PluginManifest, host: HostServices): Readonly<PluginContext> {
  const ctx: Record<string, unknown> = {
    cluster: Object.freeze({name: host.clusterName, version: host.clusterVersion}),
    namespace: host.namespace,
  };

  if ((manifest.permissions?.resources?.length ?? 0) > 0) {
    ctx.k8s = Object.freeze({
      list: (gvr: string, ns?: string) => {
        assertGVRPermission(manifest, gvr, "list");
        return host.listResources(gvr, ns);
      },
      get: (gvr: string, ns: string, name: string) => {
        assertGVRPermission(manifest, gvr, "get");
        return host.getResource(gvr, ns, name);
      },
      watch: (gvr: string, ns: string, callback: (event: unknown) => void) => {
        assertGVRPermission(manifest, gvr, "watch");
        const clusterName = host.clusterName;
        const watchNs = ns ?? "";
        const eventName = `watch:${clusterName}:${gvr}:${watchNs}`;
        // Subscribe before calling StartWatch so events fired between those
        // two steps aren't dropped.
        const unsub = Events.On(eventName, (wailsEvent: unknown) => callback((wailsEvent as {data: unknown}).data));
        // Plugins don't pre-list, so watch starts from "now" (empty RV).
        StartWatch(clusterName, gvr, watchNs, "");
        return () => {
          unsub();
          StopWatch(clusterName, gvr, watchNs);
        };
      },
    });
  }

  if (manifest.permissions?.events) {
    ctx.events = Object.freeze({
      subscribe: (eventName: string, cb: (payload: unknown) => void) =>
        Events.On("plugin:event", (wailsEvent: unknown) => {
          const data = (wailsEvent as {data?: unknown})?.data as {eventName: string; payload: unknown} | undefined;
          if (data?.eventName === eventName) {
            cb(data.payload);
          }
        }),
    });
  }

  if (manifest.permissions?.logs) {
    ctx.logs = Object.freeze({
      stream: (pod: string, ns: string, container: string, opts?: {follow?: boolean; previous?: boolean; tailLines?: number}) =>
        StartLogStream(host.clusterName, ns, pod, {
          container,
          follow: opts?.follow ?? false,
          previous: opts?.previous ?? false,
          tailLines: opts?.tailLines == null ? null : opts.tailLines,
          timestamps: false,
        } as Parameters<typeof StartLogStream>[3]),
      stop: (streamID: string) => StopLogStream(streamID),
    });
  }

  if (manifest.permissions?.exec) {
    ctx.exec = Object.freeze({
      open: (pod: string, ns: string, container: string, shell?: string) =>
        OpenExecSession(host.clusterName, ns, pod, container, shell ?? "/bin/sh"),
      close: (sessionID: string) => CloseExecSession(sessionID),
    });
  }

  if (manifest.permissions?.storage) {
    const pluginName = manifest.name;
    ctx.storage = Object.freeze({
      get: (key: string) => GetPluginStorageKey(pluginName, key),
      set: (key: string, value: string) => SetPluginStorageKey(pluginName, key, value),
      delete: (key: string) => DeletePluginStorageKey(pluginName, key),
    });
  }

  return Object.freeze(ctx as unknown as PluginContext);
}
