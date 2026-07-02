import {describe, it, expect, beforeEach, vi} from "vitest";
import {render, screen} from "@testing-library/svelte";
import Header from "$lib/components/Header.svelte";
import {clusterStore} from "$lib/stores/cluster.svelte";

// Mock the bindings used transitively
vi.mock("$api/github.com/Vilsol/klados/internal/services/clusterservice", () => ({
  ListContexts: vi.fn().mockResolvedValue([]),
  Connect: vi.fn().mockResolvedValue(undefined),
  Disconnect: vi.fn().mockResolvedValue(undefined),
  ListNamespaces: vi.fn().mockResolvedValue([]),
  SwitchNamespace: vi.fn().mockResolvedValue(undefined),
  GetStatus: vi.fn().mockResolvedValue(0),
}));

describe("Header", () => {
  beforeEach(() => {
    clusterStore.activeContext = null;
    clusterStore.namespaces = {};
    clusterStore.selectedNamespaces = {};
    clusterStore.connectionStatus = {};
  });

  it('shows "No cluster selected" when no active context', () => {
    render(Header);
    expect(screen.getByText("No cluster selected")).toBeTruthy();
  });

  it("shows cluster name when active", () => {
    clusterStore.activeContext = "my-cluster";
    clusterStore.connectionStatus["my-cluster"] = "connected";

    render(Header);
    expect(screen.getByText("my-cluster")).toBeTruthy();
  });

  it("shows namespace combobox when connected with namespaces", () => {
    clusterStore.activeContext = "my-cluster";
    clusterStore.namespaces = {"my-cluster": ["default", "kube-system"]};
    clusterStore.selectedNamespaces = {"my-cluster": ["default"]};

    render(Header);
    expect(screen.getByText("default")).toBeTruthy();
  });
});
