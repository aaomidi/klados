import {describe, it, expect, beforeEach, vi} from "vitest";
import {render, screen, waitFor} from "@testing-library/svelte";
import Sidebar from "$lib/components/Sidebar.svelte";
import {sessionStore} from "$lib/stores/session.svelte";
import type {SidebarEntry} from "$api/github.com/Vilsol/klados/internal/plugin/models";

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  ListAPIResources: vi.fn().mockResolvedValue([]),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/portforwardservice.js", () => ({
  ListForwards: vi.fn().mockResolvedValue([]),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/pluginservice.js", () => ({
  GetPluginSidebarEntries: vi.fn().mockResolvedValue([]),
}));

describe("Sidebar", () => {
  beforeEach(() => {
    sessionStore.sidebarCollapsed = false;
  });

  it("renders resource groups", () => {
    render(Sidebar);

    expect(screen.getByText("Workloads")).toBeTruthy();
    expect(screen.getByText("Networking")).toBeTruthy();
    expect(screen.getByText("Config")).toBeTruthy();
    expect(screen.getByText("Storage")).toBeTruthy();
  });

  it("shows Workloads items expanded by default", () => {
    render(Sidebar);

    expect(screen.getByText("Pods")).toBeTruthy();
    expect(screen.getByText("Deployments")).toBeTruthy();
    expect(screen.getByText("StatefulSets")).toBeTruthy();
  });

  it("collapse toggles sidebar state", () => {
    sessionStore.sidebarCollapsed = false;

    sessionStore.toggleSidebar();
    expect(sessionStore.sidebarCollapsed).toBe(true);

    sessionStore.toggleSidebar();
    expect(sessionStore.sidebarCollapsed).toBe(false);
  });
});

describe("Sidebar plugin entries", () => {
  beforeEach(() => {
    sessionStore.sidebarCollapsed = false;
  });

  it("renders plugin sidebar category and label when entries are returned", async () => {
    const {GetPluginSidebarEntries} = await import("$api/github.com/Vilsol/klados/internal/services/pluginservice.js");
    vi.mocked(GetPluginSidebarEntries).mockResolvedValue([
      {category: "Security", label: "Certificates", gvr: "cert-manager.io.v1.certificates", icon: "", plugin: "cert-manager"},
    ] as unknown as SidebarEntry[]);

    render(Sidebar);

    await waitFor(() => {
      expect(screen.getByText("Security")).toBeTruthy();
    });
  });

  it("renders multiple entries under the same category", async () => {
    const {GetPluginSidebarEntries} = await import("$api/github.com/Vilsol/klados/internal/services/pluginservice.js");
    vi.mocked(GetPluginSidebarEntries).mockResolvedValue([
      {category: "Security", label: "Certificates", gvr: "cert-manager.io.v1.certificates", icon: "", plugin: "cert-manager"},
      {category: "Security", label: "Issuers", gvr: "cert-manager.io.v1.issuers", icon: "", plugin: "cert-manager"},
    ] as unknown as SidebarEntry[]);

    render(Sidebar);

    await waitFor(() => {
      expect(screen.getByText("Security")).toBeTruthy();
    });
  });

  it("shows no plugin sections when entries list is empty", async () => {
    const {GetPluginSidebarEntries} = await import("$api/github.com/Vilsol/klados/internal/services/pluginservice.js");
    vi.mocked(GetPluginSidebarEntries).mockResolvedValue([]);

    render(Sidebar);

    await waitFor(() => {
      expect(screen.queryByText("Security")).toBeNull();
    });
  });
});
