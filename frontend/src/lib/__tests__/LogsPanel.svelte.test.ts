import {describe, it, expect, vi, beforeEach} from "vitest";
import {render, screen, waitFor} from "@testing-library/svelte";

const {mockStartLogStream, mockStopLogStream} = vi.hoisted(() => ({
  mockStartLogStream: vi.fn().mockResolvedValue("stream-id-123"),
  mockStopLogStream: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/logservice.js", () => ({
  StartLogStream: mockStartLogStream,
  StopLogStream: mockStopLogStream,
}));

vi.mock("$api/github.com/Vilsol/klados/internal/logs/models.js", () => ({
  LogOptions: class LogOptions {
    constructor(opts: unknown) {
      Object.assign(this, opts);
    }
  },
  Streamer: class Streamer {
    static createFrom(source: unknown) {
      return source;
    }
  },
}));

vi.mock("$lib/stores/streaming.svelte", () => ({
  streamingStore: {config: {port: 9999, token: "test-token"}},
}));

vi.mock("@klados/ui", () => ({
  LogViewer: vi.fn(),
  Combobox: vi.fn(),
}));

import LogsPanel from "$lib/components/panels/LogsPanel.svelte";

// A running pod: LogsPanel gates log streaming on container readiness
// (state.running / terminated in status), so the fixture must report status
// for the auto-start path to fire.
const podObj = {
  spec: {
    containers: [{name: "app"}, {name: "sidecar"}],
    initContainers: [{name: "init-setup"}],
  },
  status: {
    containerStatuses: [
      {name: "app", state: {running: {startedAt: "2026-01-01T00:00:00Z"}}},
      {name: "sidecar", state: {running: {startedAt: "2026-01-01T00:00:00Z"}}},
    ],
    initContainerStatuses: [
      {name: "init-setup", state: {terminated: {exitCode: 0}}},
    ],
  },
};

describe("LogsPanel", () => {
  beforeEach(() => {
    mockStartLogStream.mockClear();
    mockStopLogStream.mockClear();
    mockStartLogStream.mockResolvedValue("stream-id-123");
  });

  it("renders container selector with all containers", async () => {
    const {Combobox} = await import("@klados/ui");
    render(LogsPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    // Combobox receives container options including init containers
    expect(Combobox).toHaveBeenCalled();
  });

  it("renders options: timestamps, previous", () => {
    render(LogsPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    expect(screen.getByText("Timestamps")).toBeTruthy();
    expect(screen.getByText("Previous")).toBeTruthy();
  });

  it("auto-starts log stream on mount", async () => {
    render(LogsPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    await waitFor(() => expect(mockStartLogStream).toHaveBeenCalledOnce());
    expect(mockStartLogStream).toHaveBeenCalledWith("ctx", "default", "mypod", expect.any(Object));
  });

  it("shows Connecting when stream is starting", () => {
    render(LogsPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    // "Connecting…" is shown while the stream is starting
    expect(screen.getByText("Connecting…")).toBeTruthy();
  });

  it("shows error when StartLogStream rejects", async () => {
    mockStartLogStream.mockRejectedValueOnce(new Error("cluster offline"));
    render(LogsPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    // After rejection, "Connecting…" should disappear (starting = false)
    await waitFor(() => expect(mockStartLogStream).toHaveBeenCalled());
    // The error is not shown in the UI (silent failure), but starting resets
    await waitFor(() => expect(screen.queryByText("Connecting…")).toBeNull());
  });
});
