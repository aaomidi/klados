export interface ControllerRef {
  apiVersion: string;
  kind: string;
  name: string;
  uid: string;
}

import type { APIResource } from "$api/github.com/Vilsol/klados/internal/cluster/index.js";
export type { APIResource };

export function getControllerRef(obj: unknown): ControllerRef | null {
  const refs = (obj as Record<string, unknown> | undefined)?.metadata;
  const ownerRefs = (refs as Record<string, unknown> | undefined)?.ownerReferences;
  if (!Array.isArray(ownerRefs)) {
    return null;
  }
  const controller = ownerRefs.find((r: Record<string, unknown>) => r.controller === true);
  if (!controller) {
    return null;
  }
  return {
    apiVersion: controller.apiVersion as string,
    kind: controller.kind as string,
    name: controller.name as string,
    uid: controller.uid as string,
  };
}

export function gvrToApiVersion(gvr: string): string {
  const parts = gvr.split(".");
  const version = parts.at(-2) ?? "";
  const group = parts.slice(0, -2).join(".");
  if (group === "core" || group === "") {
    return version;
  }
  return `${group}/${version}`;
}

export function buildKindGVRMap(resources: APIResource[]): Map<string, string> {
  const map = new Map<string, string>();
  for (const r of resources) {
    const apiVersion = gvrToApiVersion(r.gvr);
    map.set(`${apiVersion}:${r.kind}`, r.gvr);
  }
  return map;
}

export function resolveGVR(map: Map<string, string>, apiVersion: string, kind: string): string | undefined {
  return map.get(`${apiVersion}:${kind}`);
}
