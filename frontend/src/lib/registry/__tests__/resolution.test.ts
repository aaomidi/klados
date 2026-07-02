import { describe, it, expect, beforeEach, vi } from "vitest";
import type { APIResource } from "$api/github.com/Vilsol/klados/internal/cluster/index.js";

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetDescriptors: vi.fn().mockResolvedValue([]),
}));
vi.mock("$api/github.com/Vilsol/klados/internal/services/pluginservice.js", () => ({
  GetPluginDescriptors: vi.fn().mockResolvedValue([]),
}));
vi.mock("../loaded.svelte", () => ({
  setRegistryLoaded: vi.fn(),
  registryLoaded: vi.fn().mockReturnValue(true),
}));

import { descriptorRegistry } from "../index";
import type { DescriptorDef } from "../index";

const baseResource = (over: Partial<APIResource> = {}): APIResource => ({
  gvr: "example.com.v1.widgets",
  kind: "Widget",
  namespaced: true,
  subresources: { scale: false, status: false },
  printerColumns: [],
  scaleSpec: undefined,
  ...over,
}) as unknown as APIResource;

function resetRegistry() {
  const reg = descriptorRegistry as unknown as {
    descriptors: Map<string, DescriptorDef>;
    builtins: Map<string, DescriptorDef>;
    discovery: Map<string, APIResource>;
    availableGVRs: Set<string>;
  };
  reg.descriptors = new Map();
  (reg as unknown as { builtins: Map<string, DescriptorDef> }).builtins.clear();
  reg.discovery = new Map();
  reg.availableGVRs = new Set();
}

describe("DescriptorRegistry resolution order", () => {
  beforeEach(() => {
    resetRegistry();
  });

  it("returns a generated descriptor for a GVR only in discovery", () => {
    const r = baseResource({ gvr: "example.com.v1.widgets", kind: "Widget" });
    descriptorRegistry.updateDiscovery([r]);

    const d = descriptorRegistry.get("example.com.v1.widgets");
    expect(d.kind).toBe("Widget");
    expect(d.columns.some((c) => c.name === "Name")).toBe(true);
    expect(d.columns.some((c) => c.name === "Age")).toBe(true);
  });

  it("prefers built-in over discovery when both exist", () => {
    const builtin: DescriptorDef = {
      group: "example.com",
      version: "v1",
      resource: "widgets",
      kind: "WidgetBuiltin",
      gvr: "example.com.v1.widgets",
      columns: [{ name: "BuiltinCol", expr: "metadata.name", renderType: "text" }],
      overviewFields: [],
      detailPanels: [],
      actions: [],
    };
    const reg = descriptorRegistry as unknown as { descriptors: Map<string, DescriptorDef> };
    reg.descriptors.set("example.com.v1.widgets", builtin);

    const r = baseResource({ gvr: "example.com.v1.widgets", kind: "Widget" });
    descriptorRegistry.updateDiscovery([r]);

    const d = descriptorRegistry.get("example.com.v1.widgets");
    expect(d.kind).toBe("WidgetBuiltin");
    expect(d.columns.some((c) => c.name === "BuiltinCol")).toBe(true);
  });

  it("prefers plugin descriptor over discovery when both exist", () => {
    const plugin: DescriptorDef = {
      group: "example.com",
      version: "v1",
      resource: "widgets",
      kind: "WidgetPlugin",
      gvr: "example.com.v1.widgets",
      columns: [{ name: "PluginCol", expr: "metadata.name", renderType: "text" }],
      overviewFields: [],
      detailPanels: [],
      actions: [],
    };
    // Plugins are merged into descriptors map — simulate that here
    const reg = descriptorRegistry as unknown as { descriptors: Map<string, DescriptorDef> };
    reg.descriptors.set("example.com.v1.widgets", plugin);

    const r = baseResource({ gvr: "example.com.v1.widgets", kind: "Widget" });
    descriptorRegistry.updateDiscovery([r]);

    const d = descriptorRegistry.get("example.com.v1.widgets");
    expect(d.kind).toBe("WidgetPlugin");
    expect(d.columns.some((c) => c.name === "PluginCol")).toBe(true);
  });

  it("falls back to static fallback when no built-in, plugin, or discovery entry exists", () => {
    const d = descriptorRegistry.get("unknown.v1.things");
    // Static fallback always has Name, Namespace, Age columns
    expect(d.columns.map((c) => c.name)).toContain("Name");
    expect(d.columns.map((c) => c.name)).toContain("Age");
    expect(d.kind).toBe("");
  });
});
