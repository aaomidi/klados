import {describe, it, expect, beforeEach, afterEach, vi} from "vitest";
import {render, screen, fireEvent, cleanup, waitFor} from "@testing-library/svelte";
import {tick} from "svelte";
import {APIResource} from "$api/github.com/Vilsol/klados/internal/cluster/index.js";

const mockList = vi.hoisted(() => vi.fn());
const mockListDiscoveryGVRs = vi.hoisted(() => vi.fn());
const mockPush = vi.hoisted(() => vi.fn());

vi.mock("$lib/registry/index", () => ({
  descriptorRegistry: {
    list: mockList,
    listDiscoveryGVRs: mockListDiscoveryGVRs,
  },
}));

vi.mock("svelte-spa-router", () => ({
  push: mockPush,
}));

vi.mock("$lib/plugins/slots.svelte.js", () => ({
  slotRegistry: {getCommands: vi.fn().mockReturnValue([])},
}));

vi.mock("$lib/stores/createResource.svelte", () => ({
  createResourceStore: {openDialog: vi.fn()},
}));

vi.mock("$lib/stores/applyManifest.svelte", () => ({
  applyManifestStore: {openDialog: vi.fn()},
}));

vi.mock("$lib/stores/volumeBrowser.svelte", () => ({
  volumeBrowserStore: {spawn: vi.fn()},
}));

vi.mock("$lib/stores/notification.svelte", () => ({
  notificationStore: {push: vi.fn()},
}));

vi.mock("$lib/utils/focusedPVC", () => ({
  focusedPVC: vi.fn().mockReturnValue(null),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/clusterservice.js", () => ({
  ListContexts: vi.fn().mockResolvedValue([]),
  Connect: vi.fn().mockResolvedValue(undefined),
  Disconnect: vi.fn().mockResolvedValue(undefined),
  ListNamespaces: vi.fn().mockResolvedValue([]),
}));

import {clusterStore} from "$lib/stores/cluster.svelte";
import CommandPalette from "$lib/components/CommandPalette.svelte";

const fakePod = {
  group: "",
  version: "v1",
  resource: "pods",
  kind: "Pod",
  gvr: "core.v1.pods",
  columns: [],
  overviewFields: [],
  detailPanels: [],
  actions: [],
  clusterScoped: false,
};

const fakeVirtualService = new APIResource({
  kind: "VirtualService",
  gvr: "networking.istio.io.v1.virtualservices",
  namespaced: true,
});

describe("CommandPalette", () => {
  beforeEach(() => {
    mockList.mockReturnValue([fakePod]);
    mockListDiscoveryGVRs.mockReturnValue([fakeVirtualService]);
    clusterStore.activeContext = "test-cluster";
    clusterStore.contexts = [];
    clusterStore.connectionStatus = {};
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders Navigate and Custom Resources categories in that order", async () => {
    render(CommandPalette, {props: {open: true}});
    await tick();

    await waitFor(() => {
      expect(screen.getByText("Navigate")).toBeTruthy();
      expect(screen.getByText("Custom Resources")).toBeTruthy();
    });

    const headers = screen.getAllByText(/^(Navigate|Custom Resources)$/);
    const labels = headers.map((h) => h.textContent?.trim());
    const navIdx = labels.indexOf("Navigate");
    const crdIdx = labels.indexOf("Custom Resources");
    expect(navIdx).toBeGreaterThanOrEqual(0);
    expect(crdIdx).toBeGreaterThan(navIdx);
  });

  it("renders VirtualService entry under Custom Resources", async () => {
    render(CommandPalette, {props: {open: true}});
    await tick();

    await waitFor(() => {
      expect(screen.getByText("VirtualService")).toBeTruthy();
    });
  });

  it("clicking a CRD entry calls push with the correct route", async () => {
    render(CommandPalette, {props: {open: true}});
    await tick();

    await waitFor(() => {
      expect(screen.getByText("VirtualService")).toBeTruthy();
    });

    const btn = screen.getByText("VirtualService").closest("button") as HTMLButtonElement;
    await fireEvent.click(btn);

    expect(mockPush).toHaveBeenCalledWith("/c/test-cluster/networking.istio.io.v1.virtualservices");
  });
});
