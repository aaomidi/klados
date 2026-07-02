import {describe, it, expect, vi, beforeEach} from "vitest";

vi.mock("$api/github.com/Vilsol/klados/internal/services/pluginservice.js", () => ({
  GetPluginDetailTabs: vi.fn().mockResolvedValue([]),
  GetPluginCommands: vi.fn().mockResolvedValue([]),
  GetPluginOverviewFields: vi.fn().mockResolvedValue([]),
  GetPluginListColumns: vi.fn().mockResolvedValue([]),
  GetPluginContextMenuItems: vi.fn().mockResolvedValue([]),
  GetPluginHeaderWidgets: vi.fn().mockResolvedValue([]),
  GetPluginStatusBarWidgets: vi.fn().mockResolvedValue([]),
}));

import {GetPluginDetailTabs, GetPluginCommands} from "$api/github.com/Vilsol/klados/internal/services/pluginservice.js";
import type {PermsSummary} from "$api/github.com/Vilsol/klados/internal/plugin/models.js";
import {slotRegistry} from "$lib/plugins/slots.svelte.js";

describe("SlotRegistry", () => {
  beforeEach(() => {
    // Reset state
    slotRegistry.detailTabs = [];
    slotRegistry.commands = [];
    vi.mocked(GetPluginDetailTabs).mockResolvedValue([]);
    vi.mocked(GetPluginCommands).mockResolvedValue([]);
  });

  it("getDetailTabs returns empty array for unknown GVR", () => {
    expect(slotRegistry.getDetailTabs("apps.v1.deployments")).toEqual([]);
  });

  it("initFromBackend populates detailTabs", async () => {
    vi.mocked(GetPluginDetailTabs).mockResolvedValue([
      {
        pluginName: "cert-manager",
        gvr: "cert-manager.io.v1.certificates",
        id: "cert-status",
        label: "Status",
        component: "ui/CertStatus.js",
      },
    ] as never);

    await slotRegistry.initFromBackend();

    const tabs = slotRegistry.getDetailTabs("cert-manager.io.v1.certificates");
    expect(tabs).toHaveLength(1);
    expect(tabs[0].label).toBe("Status");
    expect(tabs[0].pluginName).toBe("cert-manager");
  });

  it("getDetailTabs filters by GVR", async () => {
    vi.mocked(GetPluginDetailTabs).mockResolvedValue([
      {pluginName: "p1", gvr: "apps.v1.deployments", id: "tab1", label: "Tab 1", component: "ui.js"},
      {pluginName: "p2", gvr: "core.v1.pods", id: "tab2", label: "Tab 2", component: "ui.js"},
    ] as never);

    await slotRegistry.initFromBackend();

    expect(slotRegistry.getDetailTabs("apps.v1.deployments")).toHaveLength(1);
    expect(slotRegistry.getDetailTabs("core.v1.pods")).toHaveLength(1);
    expect(slotRegistry.getDetailTabs("apps.v1.statefulsets")).toHaveLength(0);
  });

  it("initFromBackend populates commands", async () => {
    vi.mocked(GetPluginCommands).mockResolvedValue([
      {pluginName: "my-plugin", id: "rotate", label: "Rotate Certificate", icon: null},
    ] as never);

    await slotRegistry.initFromBackend();

    const cmds = slotRegistry.getCommands();
    expect(cmds).toHaveLength(1);
    expect(cmds[0].label).toBe("Rotate Certificate");
    expect(cmds[0].pluginName).toBe("my-plugin");
  });

  it("initFromBackend replaces existing state on reload", async () => {
    vi.mocked(GetPluginDetailTabs).mockResolvedValue([
      {pluginName: "p1", gvr: "apps.v1.deployments", id: "tab1", label: "Old", component: "ui.js"},
    ] as never);
    await slotRegistry.initFromBackend();
    expect(slotRegistry.getDetailTabs("apps.v1.deployments")).toHaveLength(1);

    vi.mocked(GetPluginDetailTabs).mockResolvedValue([]);
    await slotRegistry.initFromBackend();
    expect(slotRegistry.getDetailTabs("apps.v1.deployments")).toHaveLength(0);
  });

  it("registerCommand adds to commands list", () => {
    slotRegistry.registerCommand({pluginName: "p", id: "cmd", label: "Do Thing", perms: {} as PermsSummary, action: vi.fn()});
    expect(slotRegistry.getCommands()).toHaveLength(1);
  });
});
