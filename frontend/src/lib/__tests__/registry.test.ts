import {describe, it, expect, vi, beforeEach} from "vitest";

const mockGetDescriptors = vi.hoisted(() => vi.fn());
const mockGetPluginDescriptors = vi.hoisted(() => vi.fn());

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetDescriptors: mockGetDescriptors,
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/pluginservice.js", () => ({
  GetPluginDescriptors: mockGetPluginDescriptors,
}));

import {descriptorRegistry} from "$lib/registry/index.js";

const builtinNodes = {
  group: "",
  version: "v1",
  resource: "nodes",
  kind: "Node",
  columns: [
    {name: "Name", expr: "metadata.name", renderType: "text"},
    {name: "Status", expr: "status.readyStatus", renderType: "badge", width: 90},
    {name: "Roles", expr: "status.roles", renderType: "text", width: 130},
    {name: "Age", expr: "metadata.creationTimestamp", renderType: "age", width: 80},
  ],
  overviewFields: [],
  detailPanels: ["overview", "node", "labels", "events", "yaml"],
  actions: [],
};

describe("DescriptorRegistry — plugin column behaviour", () => {
  beforeEach(() => {
    mockGetDescriptors.mockResolvedValue([builtinNodes]);
    mockGetPluginDescriptors.mockResolvedValue([]);
  });

  it("without plugins the builtin columns are returned", async () => {
    await descriptorRegistry.load();
    const desc = descriptorRegistry.get("core.v1.nodes");
    expect(desc.columns.map((c) => c.name)).toEqual(["Name", "Status", "Roles", "Controlled By", "Age"]);
  });

  it("plugin columns replace builtin columns when plugin provides non-empty columns", async () => {
    mockGetPluginDescriptors.mockResolvedValue([
      {
        group: "",
        version: "v1",
        resource: "nodes",
        kind: "Node",
        columns: [
          {name: "Name", expr: "metadata.name", renderType: "text"},
          {name: "Status", expr: "status.readinessSummary", renderType: "badge"},
          {name: "Taints", expr: "has(status.taintCount) ? string(status.taintCount) : '0'", renderType: "text"},
          {name: "Age", expr: "metadata.creationTimestamp", renderType: "age"},
        ],
        overviewFields: [],
        detailPanels: [],
        actions: [],
      },
    ]);

    await descriptorRegistry.load();

    const desc = descriptorRegistry.get("core.v1.nodes");
    expect(desc.columns.map((c) => c.name)).toEqual(["Name", "Status", "Taints", "Controlled By", "Age"]);
    expect(desc.columns.find((c) => c.name === "Taints")?.expr).toBe("has(status.taintCount) ? string(status.taintCount) : '0'");
    expect(desc.columns.find((c) => c.name === "Roles")).toBeUndefined();
  });

  it("builtin columns are kept when plugin provides empty columns", async () => {
    mockGetPluginDescriptors.mockResolvedValue([
      {
        group: "",
        version: "v1",
        resource: "nodes",
        kind: "Node",
        columns: [],
        overviewFields: [],
        detailPanels: ["node-annotator-info"],
        actions: [],
      },
    ]);

    await descriptorRegistry.load();

    const desc = descriptorRegistry.get("core.v1.nodes");
    expect(desc.columns.map((c) => c.name)).toEqual(["Name", "Status", "Roles", "Controlled By", "Age"]);
  });

  it("plugin detail panels are appended and duplicates are dropped", async () => {
    mockGetPluginDescriptors.mockResolvedValue([
      {
        group: "",
        version: "v1",
        resource: "nodes",
        kind: "Node",
        columns: [],
        overviewFields: [],
        detailPanels: ["node-annotator-info", "overview"],
        actions: [],
      },
    ]);

    await descriptorRegistry.load();

    const panels = descriptorRegistry.get("core.v1.nodes").detailPanels;
    expect(panels).toContain("overview");
    expect(panels).toContain("node-annotator-info");
    expect(panels.filter((p) => p === "overview")).toHaveLength(1);
  });

  it("plugin descriptor for an unknown GVR is added as a new entry", async () => {
    mockGetPluginDescriptors.mockResolvedValue([
      {
        group: "cert-manager.io",
        version: "v1",
        resource: "certificates",
        kind: "Certificate",
        columns: [
          {name: "Name", expr: "metadata.name", renderType: "text"},
          {name: "Ready", expr: "status.conditions", renderType: "badge"},
        ],
        overviewFields: [],
        detailPanels: [],
        actions: [],
      },
    ]);

    await descriptorRegistry.load();

    const desc = descriptorRegistry.get("cert-manager.io.v1.certificates");
    expect(desc.columns.map((c) => c.name)).toEqual(["Name", "Ready", "Controlled By"]);
  });

  it("reloadPlugins resets to builtins then re-merges", async () => {
    mockGetPluginDescriptors.mockResolvedValue([
      {
        group: "",
        version: "v1",
        resource: "nodes",
        kind: "Node",
        columns: [{name: "Custom", expr: "status.custom", renderType: "text"}],
        overviewFields: [],
        detailPanels: [],
        actions: [],
      },
    ]);
    await descriptorRegistry.load();
    expect(descriptorRegistry.get("core.v1.nodes").columns.map((c) => c.name)).toEqual(["Custom", "Controlled By"]);

    mockGetPluginDescriptors.mockResolvedValue([]);
    await descriptorRegistry.reloadPlugins();
    expect(descriptorRegistry.get("core.v1.nodes").columns.map((c) => c.name)).toEqual(["Name", "Status", "Roles", "Controlled By", "Age"]);
  });

  describe("nodes.yaml descriptor format", () => {
    it("descriptor with wrong field names (gvr/defaultColumns) produces no match — exposes format bug", async () => {
      // This is what the current nodes.yaml actually produces after Go JSON unmarshalling:
      // - group/version/resource are empty because nodes.yaml uses "gvr" not separate fields
      // - columns is null because nodes.yaml uses "defaultColumns" not "columns"
      mockGetPluginDescriptors.mockResolvedValue([
        {
          group: "",
          version: "",
          resource: "",
          kind: "",
          columns: null, // defaultColumns is not a known field
          overviewFields: null,
          detailPanels: null,
          actions: null,
        },
      ]);

      await descriptorRegistry.load();

      // With wrong field names, the GVR computes to "core.." — does NOT match "core.v1.nodes"
      const desc = descriptorRegistry.get("core.v1.nodes");
      // Builtin columns are returned unchanged — plugin descriptor was unrecognised
      expect(desc.columns.map((c) => c.name)).toEqual(["Name", "Status", "Roles", "Controlled By", "Age"]);
    });

    it("descriptor with correct field names works", async () => {
      // This is what nodes.yaml SHOULD produce — group/version/resource populated, columns not defaultColumns
      mockGetPluginDescriptors.mockResolvedValue([
        {
          group: "",
          version: "v1",
          resource: "nodes",
          kind: "Node",
          columns: [
            {name: "Name", expr: "metadata.name", renderType: "text"},
            {name: "Status", expr: "status.readinessSummary", renderType: "badge"},
            {name: "Taints", expr: "has(status.taintCount) ? string(status.taintCount) : '0'", renderType: "text"},
            {name: "Age", expr: "metadata.creationTimestamp", renderType: "age"},
          ],
          overviewFields: [],
          detailPanels: [],
          actions: [],
        },
      ]);

      await descriptorRegistry.load();

      const desc = descriptorRegistry.get("core.v1.nodes");
      expect(desc.columns.map((c) => c.name)).toEqual(["Name", "Status", "Taints", "Controlled By", "Age"]);
    });
  });
});
