import {describe, it, expect, vi, beforeEach} from "vitest";
import {render, screen, waitFor} from "@testing-library/svelte";

const {mockGetResource} = vi.hoisted(() => ({
  mockGetResource: vi.fn(),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({GetResource: mockGetResource}));

vi.mock("@klados/ui", () => ({
  SectionHeader: vi.fn(),
  KeyValueBadge: vi.fn(),
  EmptyState: vi.fn(),
}));

import ServicePanel from "$lib/components/panels/ServicePanel.svelte";

const obj = {
  metadata: {name: "my-service", namespace: "default"},
  spec: {
    type: "ClusterIP",
    selector: {app: "myapp", tier: "frontend"},
    ports: [{name: "http", port: 80, protocol: "TCP", targetPort: 8080}],
  },
};

const endpointsObj = {
  subsets: [
    {
      addresses: [
        {ip: "10.0.0.1", targetRef: {name: "myapp-abc"}},
        {ip: "10.0.0.2", targetRef: {name: "myapp-def"}},
      ],
    },
  ],
};

describe("ServicePanel", () => {
  beforeEach(() => {
    mockGetResource.mockResolvedValue(endpointsObj);
  });

  it("renders selector labels", async () => {
    const {KeyValueBadge} = await import("@klados/ui");
    render(ServicePanel, {props: {obj, ctxName: "ctx1"}});
    // KeyValueBadge receives the selector entries
    expect(KeyValueBadge).toHaveBeenCalled();
  });

  it("renders port table", () => {
    render(ServicePanel, {props: {obj, ctxName: "ctx1"}});
    expect(screen.getByText("http")).toBeTruthy();
    expect(screen.getByText("80")).toBeTruthy();
  });

  it("shows loading state while fetching endpoints", async () => {
    const {EmptyState} = await import("@klados/ui");
    mockGetResource.mockReturnValue(
      new Promise(() => {
        /* empty */
      }),
    ); // never resolves
    render(ServicePanel, {props: {obj, ctxName: "ctx1"}});
    expect(EmptyState).toHaveBeenCalled();
  });

  it("shows backing pods after endpoints load", async () => {
    render(ServicePanel, {props: {obj, ctxName: "ctx1"}});
    await waitFor(() => {
      expect(screen.getByText("myapp-abc")).toBeTruthy();
      expect(screen.getByText("myapp-def")).toBeTruthy();
    });
  });

  it("shows IP addresses for endpoints", async () => {
    render(ServicePanel, {props: {obj, ctxName: "ctx1"}});
    await waitFor(() => {
      expect(screen.getByText("10.0.0.1")).toBeTruthy();
    });
  });

  it("calls GetResource with correct endpoint GVR", async () => {
    render(ServicePanel, {props: {obj, ctxName: "ctx1"}});
    await waitFor(() => {
      expect(mockGetResource).toHaveBeenCalledWith("ctx1", "core.v1.endpoints", "default", "my-service");
    });
  });

  it("shows no endpoints message when none found", async () => {
    const {EmptyState} = await import("@klados/ui");
    mockGetResource.mockResolvedValue({subsets: []});
    render(ServicePanel, {props: {obj, ctxName: "ctx1"}});
    await waitFor(() => {
      expect(EmptyState).toHaveBeenCalled();
    });
  });

  it("shows no selector message when empty", async () => {
    const {EmptyState} = await import("@klados/ui");
    const noSelectorObj = {metadata: {name: "svc", namespace: "ns"}, spec: {selector: {}}};
    render(ServicePanel, {props: {obj: noSelectorObj, ctxName: "ctx1"}});
    expect(EmptyState).toHaveBeenCalled();
  });
});
