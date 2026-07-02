import {describe, it, expect, vi, beforeEach} from "vitest";
import {render, screen, waitFor, fireEvent} from "@testing-library/svelte";

const {mockOpenExecSession, mockCloseExecSession, mockStop, mockReplace, mockSpawn, mockAttachTab, mockGetResource} = vi.hoisted(() => ({
  mockOpenExecSession: vi.fn().mockResolvedValue("session-id-abc"),
  mockCloseExecSession: vi.fn().mockResolvedValue(undefined),
  mockStop: vi.fn().mockResolvedValue(undefined),
  mockReplace: vi.fn(),
  mockSpawn: vi.fn(),
  mockAttachTab: vi.fn().mockResolvedValue(undefined),
  mockGetResource: vi.fn().mockResolvedValue({}),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/execservice.js", () => ({
  OpenExecSession: mockOpenExecSession,
  CloseExecSession: mockCloseExecSession,
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetResource: (...a: unknown[]) => mockGetResource(...a),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/volumebrowserservice.js", () => ({
  Stop: (...a: unknown[]) => mockStop(...a),
  Replace: (...a: unknown[]) => mockReplace(...a),
  Spawn: (...a: unknown[]) => mockSpawn(...a),
  AttachTab: (...a: unknown[]) => mockAttachTab(...a),
}));

vi.mock("$lib/stores/streaming.svelte", () => ({
  streamingStore: {config: {port: 9999, token: "test-token"}},
}));

vi.mock("@klados/ui", () => ({
  Terminal: vi.fn(),
  Combobox: vi.fn(),
}));

import TerminalPanel from "$lib/components/panels/TerminalPanel.svelte";

const podObj = {
  spec: {
    containers: [{name: "app"}, {name: "worker"}],
    initContainers: [],
  },
};

describe("TerminalPanel", () => {
  beforeEach(() => {
    mockOpenExecSession.mockClear();
    mockCloseExecSession.mockClear();
    mockOpenExecSession.mockResolvedValue("session-id-abc");
  });

  it("renders container selector", async () => {
    const {Combobox} = await import("@klados/ui");
    render(TerminalPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    // Combobox receives container options
    expect(Combobox).toHaveBeenCalled();
  });

  it("renders shell selector buttons", () => {
    render(TerminalPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    expect(screen.getByText("bash")).toBeTruthy();
    expect(screen.getByText("sh")).toBeTruthy();
    expect(screen.getByText("zsh")).toBeTruthy();
  });

  it("renders Connect button", () => {
    render(TerminalPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    expect(screen.getByText("Connect")).toBeTruthy();
  });

  function getConnectBtn() {
    return waitFor(() => {
      const b = screen.getByText("Connect") as HTMLButtonElement;
      expect(b.disabled).toBe(false);
      return b;
    });
  }

  it("calls OpenExecSession with correct args on Connect click", async () => {
    render(TerminalPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    await fireEvent.click(await getConnectBtn());
    await waitFor(() => expect(mockOpenExecSession).toHaveBeenCalledOnce());
    expect(mockOpenExecSession).toHaveBeenCalledWith("ctx", "default", "mypod", "app", "bash");
  });

  it("shows error when OpenExecSession rejects", async () => {
    mockOpenExecSession.mockRejectedValueOnce(new Error("pod not found"));
    render(TerminalPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    await fireEvent.click(await getConnectBtn());
    await waitFor(() => expect(screen.getByText("pod not found")).toBeTruthy());
  });

  it("changes shell when clicking different shell button", async () => {
    render(TerminalPanel, {
      props: {obj: podObj, ctxName: "ctx", namespace: "default", name: "mypod"},
    });
    await fireEvent.click(screen.getByText("zsh"));
    await fireEvent.click(await getConnectBtn());
    await waitFor(() => expect(mockOpenExecSession).toHaveBeenCalledOnce());
    expect(mockOpenExecSession).toHaveBeenCalledWith("ctx", "default", "mypod", "app", "zsh");
  });
});

describe("TerminalPanel (terminal-pending lifecycle)", () => {
  it("transitions tab.kind to 'terminal' when pod is Running + Ready", async () => {
    const {resourceCache} = await import("$lib/stores/resourceCache.svelte");
    const {bottomPanelStore} = await import("$lib/stores/bottom-panel.svelte");

    // Seed resource cache with a Pending pod (ContainerCreating).
    resourceCache.upsert("ctx", "core.v1.pods", {
      metadata: {uid: "u1", namespace: "ns1", name: "pvc-browser-1"},
      status: {
        phase: "Pending",
        containerStatuses: [{name: "browser", ready: false, state: {waiting: {reason: "ContainerCreating"}}}],
      },
    });

    // Register a tab so setKind has a target.
    const tabId = bottomPanelStore.addTab({
      kind: "terminal-pending",
      ctxName: "ctx",
      gvr: "core.v1.pods",
      namespace: "ns1",
      name: "pvc-browser-1",
      resourceKind: "Pod",
      resourceName: "pvc-browser-1",
      obj: {},
      managedId: "mgd-1",
    });

    render(TerminalPanel, {
      props: {
        obj: podObj,
        ctxName: "ctx",
        namespace: "ns1",
        name: "pvc-browser-1",
        tabId,
        tabKind: "terminal-pending",
        managedId: "mgd-1",
      },
    });

    // Initial: waiting UI visible.
    await waitFor(() => expect(screen.getByTestId("pending-waiting")).toBeTruthy());

    // Transition pod to Running + Ready.
    resourceCache.upsert("ctx", "core.v1.pods", {
      metadata: {uid: "u1", namespace: "ns1", name: "pvc-browser-1"},
      status: {
        phase: "Running",
        containerStatuses: [{name: "browser", ready: true, state: {running: {startedAt: "now"}}}],
      },
    });

    await waitFor(() => {
      const tab = bottomPanelStore.tabs.find((t) => t.id === tabId);
      expect(tab?.kind).toBe("terminal");
    });

    bottomPanelStore.closeTab(tabId);
  });

  it("renders error block with Delete and Delete & Retry after stuck ImagePullBackOff", async () => {
    vi.useFakeTimers();
    const now = Date.UTC(2026, 0, 1, 12, 0, 0);
    vi.setSystemTime(now);

    const {resourceCache} = await import("$lib/stores/resourceCache.svelte");
    resourceCache.upsert("ctx", "core.v1.pods", {
      metadata: {uid: "u2", namespace: "ns1", name: "stuck-pod"},
      status: {
        phase: "Pending",
        containerStatuses: [
          {name: "browser", ready: false, state: {waiting: {reason: "ImagePullBackOff", message: "Back-off pulling image foo"}}},
        ],
      },
    });

    render(TerminalPanel, {
      props: {
        obj: podObj,
        ctxName: "ctx",
        namespace: "ns1",
        name: "stuck-pod",
        tabId: "tab-stuck",
        tabKind: "terminal-pending",
        managedId: "mgd-stuck",
      },
    });

    // Advance past the 60s stuck timeout. The $effect ticks every second.
    await vi.advanceTimersByTimeAsync(65_000);

    await waitFor(() => expect(screen.getByTestId("pending-error")).toBeTruthy());
    expect(screen.getByText("Delete")).toBeTruthy();
    expect(screen.getByText("Delete & Retry")).toBeTruthy();

    vi.useRealTimers();
  });

  it("Delete click: calls Stop exactly once via closeTab and removes the tab", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.UTC(2026, 0, 2, 12, 0, 0));
    mockStop.mockClear();

    const {resourceCache} = await import("$lib/stores/resourceCache.svelte");
    const {bottomPanelStore} = await import("$lib/stores/bottom-panel.svelte");
    resourceCache.upsert("ctx", "core.v1.pods", {
      metadata: {uid: "u-del", namespace: "ns1", name: "stuck-del"},
      status: {
        phase: "Pending",
        containerStatuses: [
          {name: "browser", ready: false, state: {waiting: {reason: "ImagePullBackOff", message: "no"}}},
        ],
      },
    });
    const tabId = bottomPanelStore.addTab({
      kind: "terminal-pending",
      ctxName: "ctx",
      gvr: "core.v1.pods",
      namespace: "ns1",
      name: "stuck-del",
      resourceKind: "Pod",
      resourceName: "stuck-del",
      obj: {},
      managedId: "mgd-del",
    });

    render(TerminalPanel, {
      props: {
        obj: podObj,
        ctxName: "ctx",
        namespace: "ns1",
        name: "stuck-del",
        tabId,
        tabKind: "terminal-pending",
        managedId: "mgd-del",
      },
    });

    await vi.advanceTimersByTimeAsync(65_000);
    await waitFor(() => expect(screen.getByText("Delete")).toBeTruthy());

    vi.useRealTimers();
    await fireEvent.click(screen.getByText("Delete"));

    await waitFor(() => expect(bottomPanelStore.tabs.find((t) => t.id === tabId)).toBeUndefined());
    expect(mockStop).toHaveBeenCalledTimes(1);
    expect(mockStop).toHaveBeenCalledWith("mgd-del");
  });

  it("Delete & Retry click: calls Replace, removes old tab, adds new terminal-pending tab", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.UTC(2026, 0, 3, 12, 0, 0));
    mockStop.mockClear();
    mockReplace.mockReset();
    mockReplace.mockResolvedValue({id: "new-mgd", namespace: "ns1", podName: "pvc-browser-new"});
    mockAttachTab.mockClear();

    const {resourceCache} = await import("$lib/stores/resourceCache.svelte");
    const {bottomPanelStore} = await import("$lib/stores/bottom-panel.svelte");
    const {volumeBrowserStore} = await import("$lib/stores/volumeBrowser.svelte");

    // Seed a known lastRequests entry so retry has the original request.
    volumeBrowserStore.lastRequests.set("mgd-retry", {
      contextName: "ctx",
      namespace: "ns1",
      pvcName: "data-pvc",
    } as unknown as Parameters<typeof volumeBrowserStore.lastRequests.set>[1]);

    resourceCache.upsert("ctx", "core.v1.pods", {
      metadata: {uid: "u-retry", namespace: "ns1", name: "stuck-retry"},
      status: {
        phase: "Pending",
        containerStatuses: [
          {name: "browser", ready: false, state: {waiting: {reason: "ImagePullBackOff", message: "no"}}},
        ],
      },
    });

    const oldTabId = bottomPanelStore.addTab({
      kind: "terminal-pending",
      ctxName: "ctx",
      gvr: "core.v1.pods",
      namespace: "ns1",
      name: "stuck-retry",
      resourceKind: "Pod",
      resourceName: "stuck-retry",
      obj: {},
      managedId: "mgd-retry",
    });

    render(TerminalPanel, {
      props: {
        obj: podObj,
        ctxName: "ctx",
        namespace: "ns1",
        name: "stuck-retry",
        tabId: oldTabId,
        tabKind: "terminal-pending",
        managedId: "mgd-retry",
      },
    });

    await vi.advanceTimersByTimeAsync(65_000);
    await waitFor(() => expect(screen.getByText("Delete & Retry")).toBeTruthy());

    vi.useRealTimers();
    await fireEvent.click(screen.getByText("Delete & Retry"));

    await waitFor(() => expect(mockReplace).toHaveBeenCalledTimes(1));
    expect(mockReplace).toHaveBeenCalledWith("mgd-retry", expect.objectContaining({contextName: "ctx", pvcName: "data-pvc"}));

    await waitFor(() => expect(bottomPanelStore.tabs.find((t) => t.id === oldTabId)).toBeUndefined());
    // A new terminal-pending tab for the new managedId should have been added.
    await waitFor(() => {
      const newTab = bottomPanelStore.tabs.find((t) => t.managedId === "new-mgd");
      expect(newTab).toBeTruthy();
      expect(newTab?.kind).toBe("terminal-pending");
    });
    // Server-side Replace already tore down the old pod — no Stop RPC for old managedId.
    expect(mockStop).not.toHaveBeenCalledWith("mgd-retry");

    // Cleanup
    const newTab = bottomPanelStore.tabs.find((t) => t.managedId === "new-mgd");
    if (newTab) bottomPanelStore.closeTab(newTab.id, {skipStop: true});
  });
});
