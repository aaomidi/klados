import {describe, it, expect, vi, beforeEach} from "vitest";
import {render, screen, fireEvent, waitFor} from "@testing-library/svelte";
import {Events as WailsEvents} from "@wailsio/runtime";

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetEvents: vi.fn().mockResolvedValue([]),
  StartWatch: vi.fn().mockResolvedValue(undefined),
  StopWatch: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("$lib/stores/cluster.svelte", () => ({
  clusterStore: {
    resolveOwnerGVR: vi.fn().mockReturnValue("core.v1.pods"),
  },
}));

import EventsPanel from "$lib/components/panels/EventsPanel.svelte";
import {GetEvents, StartWatch, StopWatch} from "$api/github.com/Vilsol/klados/internal/services/resourceservice.js";

const mockGetEvents = GetEvents as ReturnType<typeof vi.fn>;
const mockStartWatch = StartWatch as ReturnType<typeof vi.fn>;
const mockStopWatch = StopWatch as ReturnType<typeof vi.fn>;
const mockEventsOn = WailsEvents.On as ReturnType<typeof vi.fn>;

function watchHandler(key: string): ((e: unknown) => void) | undefined {
  const call = mockEventsOn.mock.calls.find((c: unknown[]) => c[0] === key);
  return call?.[1] as (e: unknown) => void;
}

describe("EventsPanel", () => {
  const defaultEvents = [
    {
      type: "Normal",
      reason: "Pulled",
      message: 'Successfully pulled image "nginx"',
      count: 3,
      lastTimestamp: new Date(Date.now() - 600 * 1000).toISOString(),
      source: {component: "kubelet"},
      involvedObject: {uid: "abc-123"},
      metadata: {uid: "ev-1", namespace: "default", creationTimestamp: new Date().toISOString()},
    },
    {
      type: "Warning",
      reason: "BackOff",
      message: "Back-off restarting failed container",
      count: 10,
      lastTimestamp: new Date(Date.now() - 60 * 1000).toISOString(),
      source: {component: "kubelet"},
      involvedObject: {uid: "abc-123"},
      metadata: {uid: "ev-2", namespace: "default"},
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
    mockEventsOn.mockReturnValue(vi.fn());
    mockGetEvents.mockResolvedValue(defaultEvents);
  });

  it("renders loading state initially", () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    expect(screen.getByText("Loading events...")).toBeTruthy();
  });

  it("renders event rows after load", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("Pulled")).toBeTruthy());
    expect(screen.getByText("BackOff")).toBeTruthy();
  });

  it("shows empty state when no events", async () => {
    mockGetEvents.mockResolvedValue([]);
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("No events found.")).toBeTruthy());
  });

  it("calls GetEvents with empty namespace for cluster-scoped resources", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "", uid: "cluster-uid"}});
    await waitFor(() => expect(mockGetEvents).toHaveBeenCalledWith("ctx", "", "cluster-uid"));
  });

  it("applies amber tint to Warning event rows only", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("BackOff")).toBeTruthy());
    const rows = Array.from(document.querySelectorAll("tbody tr"));
    const warningRow = rows.find((r) => r.textContent?.includes("BackOff"));
    const normalRow = rows.find((r) => r.textContent?.includes("Pulled"));
    expect(warningRow?.className).toContain("bg-amber-500/5");
    expect(normalRow?.className).not.toContain("bg-amber-500/5");
  });

  it("starts an events watch on mount and stops it on unmount", async () => {
    const {unmount} = render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(mockStartWatch).toHaveBeenCalledWith("ctx", "core.v1.events", "default", ""));
    unmount();
    expect(mockStopWatch).toHaveBeenCalledWith("ctx", "core.v1.events", "default");
  });

  it("applies a matching ADDED watch event without refetching", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("Pulled")).toBeTruthy());

    const handler = watchHandler("watch:ctx:core.v1.events:default");
    expect(handler).toBeTruthy();
    handler?.({
      data: {
        type: "ADDED",
        object: {
          type: "Warning",
          reason: "FailedScheduling",
          message: "0/3 nodes are available",
          involvedObject: {uid: "abc-123"},
          lastTimestamp: new Date().toISOString(),
          metadata: {uid: "ev-3", namespace: "default"},
        },
      },
    });

    await waitFor(() => expect(screen.getByText("FailedScheduling")).toBeTruthy());
    expect(mockGetEvents).toHaveBeenCalledTimes(1);
  });

  it("upserts an existing event on MODIFIED instead of duplicating", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("BackOff")).toBeTruthy());

    const handler = watchHandler("watch:ctx:core.v1.events:default");
    handler?.({
      data: {
        type: "MODIFIED",
        object: {...defaultEvents[1], count: 11, lastTimestamp: new Date().toISOString()},
      },
    });

    await waitFor(() => expect(screen.getByText("11")).toBeTruthy());
    expect(screen.getAllByText("BackOff")).toHaveLength(1);
  });

  it("ignores watch events for other objects", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("Pulled")).toBeTruthy());

    const handler = watchHandler("watch:ctx:core.v1.events:default");
    handler?.({
      data: {
        type: "ADDED",
        object: {
          reason: "SomethingElse",
          involvedObject: {uid: "other-uid"},
          metadata: {uid: "ev-x", namespace: "default"},
        },
      },
    });

    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByText("SomethingElse")).toBeNull();
  });

  it("sorts events by last seen descending", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("Pulled")).toBeTruthy());
    const rows = Array.from(document.querySelectorAll("tbody tr"));
    // BackOff (60s ago) is more recent than Pulled (600s ago)
    expect(rows[0].textContent).toContain("BackOff");
    expect(rows[1].textContent).toContain("Pulled");
  });

  it("shows First seen, Last seen and Source columns", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("Pulled")).toBeTruthy());
    expect(screen.getByText("First seen")).toBeTruthy();
    expect(screen.getByText("Last seen")).toBeTruthy();
    expect(screen.getByText("Source")).toBeTruthy();
    expect(screen.getAllByText("kubelet").length).toBeGreaterThan(0);
  });

  it("filters to warnings only when the warnings chip is clicked", async () => {
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123"}});
    await waitFor(() => expect(screen.getByText("Pulled")).toBeTruthy());

    await fireEvent.click(screen.getByRole("button", {name: /warnings/i}));
    expect(screen.queryByText("Pulled")).toBeNull();
    expect(screen.getByText("BackOff")).toBeTruthy();

    await fireEvent.click(screen.getByRole("button", {name: /warnings/i}));
    expect(screen.getByText("Pulled")).toBeTruthy();
  });

  it("linkifies objects referenced in messages and calls onopenresource", async () => {
    mockGetEvents.mockResolvedValue([
      {
        type: "Normal",
        reason: "SuccessfulCreate",
        message: "Created pod: web-6d4cf56db6-x7k2p",
        involvedObject: {uid: "abc-123"},
        lastTimestamp: new Date().toISOString(),
        metadata: {uid: "ev-1", namespace: "default"},
      },
    ]);
    const onopenresource = vi.fn();
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123", onopenresource}});
    await waitFor(() => expect(screen.getByText("web-6d4cf56db6-x7k2p")).toBeTruthy());

    await fireEvent.click(screen.getByText("web-6d4cf56db6-x7k2p"));
    expect(onopenresource).toHaveBeenCalledWith("core.v1.pods", "default", "web-6d4cf56db6-x7k2p");
  });

  it("linkifies cluster-scoped node references with empty namespace", async () => {
    mockGetEvents.mockResolvedValue([
      {
        type: "Normal",
        reason: "Scheduled",
        message: "Successfully assigned default/web-abc to node-1",
        involvedObject: {uid: "abc-123"},
        lastTimestamp: new Date().toISOString(),
        metadata: {uid: "ev-1", namespace: "default"},
      },
    ]);
    const onopenresource = vi.fn();
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123", onopenresource}});
    await waitFor(() => expect(screen.getByText("node-1")).toBeTruthy());

    await fireEvent.click(screen.getByText("node-1"));
    expect(onopenresource).toHaveBeenCalledWith("core.v1.nodes", "", "node-1");
  });

  it("links the related object reference when present", async () => {
    mockGetEvents.mockResolvedValue([
      {
        type: "Normal",
        reason: "Something",
        message: "A message",
        involvedObject: {uid: "abc-123"},
        related: {kind: "Pod", apiVersion: "v1", name: "web-1", namespace: "default"},
        lastTimestamp: new Date().toISOString(),
        metadata: {uid: "ev-1", namespace: "default"},
      },
    ]);
    const onopenresource = vi.fn();
    render(EventsPanel, {props: {ctxName: "ctx", namespace: "default", uid: "abc-123", onopenresource}});
    await waitFor(() => expect(screen.getByText("Pod/web-1")).toBeTruthy());

    await fireEvent.click(screen.getByText("Pod/web-1"));
    expect(onopenresource).toHaveBeenCalledWith("core.v1.pods", "default", "web-1");
  });
});
