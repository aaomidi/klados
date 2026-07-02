import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/svelte";

// Mock heavy dependencies before importing the component
vi.mock("svelte-spa-router", () => ({ push: vi.fn(), default: vi.fn() }));

vi.mock("$lib/stores/cluster.svelte", () => ({
  clusterStore: {
    contexts: [{ name: "my-cluster", cluster: "my-cluster", user: "admin", namespace: "default" }],
    connectionStatus: {},
    activeContext: null,
    selectedNamespaces: [],
    namespaces: [],
    connect: vi.fn(),
    disconnect: vi.fn(),
    setActiveContext: vi.fn(),
  },
}));

vi.mock("$lib/components/DataTable.svelte", () => ({
  default: vi.fn(),
}));

vi.mock("$lib/components/ColumnMenu.svelte", () => ({
  default: vi.fn(),
}));

vi.mock("$lib/components/KubeconfigImportDialog.svelte", () => ({
  default: vi.fn(),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/configservice.js", () => ({
  GetColumnPrefs: vi.fn().mockResolvedValue(null),
  SetColumnPrefs: vi.fn().mockResolvedValue(undefined),
  DeleteColumnPrefs: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/config/models.js", () => ({
  GVRColumnPrefs: vi.fn(),
  ColumnSettings: vi.fn(),
  SortPrefs: vi.fn(),
}));

describe("ClusterList gear button", () => {
  it("rowSuffix contains a Settings gear button with correct aria-label", async () => {
    const src = await import("../ClusterList.svelte?raw");
    const source: string = (src as { default: string }).default;
    expect(source).toContain('aria-label="Settings for {ctx.name}"');
    expect(source).toContain("Settings size={13}");
    expect(source).toContain('/settings/clusters/${encodeURIComponent(ctx.name)}');
  });

  it("gear button uses e.stopPropagation", async () => {
    const src = await import("../ClusterList.svelte?raw");
    const source: string = (src as { default: string }).default;
    const gearButtonMatch = source.match(/aria-label="Settings for \{ctx\.name\}"[\s\S]{0,500}?<\/button>/);
    expect(gearButtonMatch).toBeTruthy();
    // The onclick before the gear button should stopPropagation
    const gearSection = source.slice(source.indexOf('aria-label="Settings for {ctx.name}"') - 300, source.indexOf('aria-label="Settings for {ctx.name}"') + 50);
    expect(gearSection).toContain("stopPropagation");
  });
});
