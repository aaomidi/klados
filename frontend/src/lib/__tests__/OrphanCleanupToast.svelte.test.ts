import {describe, it, expect, beforeEach, vi} from "vitest";
import {render} from "@testing-library/svelte";
import {tick} from "svelte";

const triggerMock = vi.fn();
const cleanupMock = vi.fn();
const pushMock = vi.fn();
const dismissMock = vi.fn();

vi.mock("$api/github.com/Vilsol/klados/internal/services/volumebrowserservice.js", () => ({
  TriggerOrphanScan: (...args: unknown[]) => triggerMock(...args),
  CleanupOrphans: (...args: unknown[]) => cleanupMock(...args),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/cluster/models.js", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return {
    ...actual,
    ConnectionStatus: {
      StatusDisconnected: 0,
      StatusConnecting: 1,
      StatusConnected: 2,
      StatusError: 3,
    },
  };
});

// Avoid loading the real cluster store (it pulls codemirror-json-schema via
// transitive imports from other components). vi.mock is hoisted, so use a
// top-level-safe shared object.
vi.mock("$lib/stores/cluster.svelte", () => {
  const state: {contexts: Array<Record<string, unknown>>} = {contexts: []};
  return {
    clusterStore: state,
    // Expose the underlying state for tests to mutate.
    __fake: state,
  };
});

vi.mock("$lib/stores/notification.svelte", () => ({
  notificationStore: {
    push: (...args: unknown[]) => {
      pushMock(...args);
      return "toast-id";
    },
    dismiss: (...args: unknown[]) => dismissMock(...args),
    error: vi.fn(),
  },
}));

import OrphanCleanupToast from "$lib/components/OrphanCleanupToast.svelte";
import {clusterStore} from "$lib/stores/cluster.svelte";

const fakeCluster = clusterStore as unknown as {contexts: Array<Record<string, unknown>>};

function connectedCtx(name: string) {
  return {name, cluster: "", user: "", namespace: "", status: 2};
}

describe("OrphanCleanupToast", () => {
  beforeEach(() => {
    triggerMock.mockReset();
    cleanupMock.mockReset();
    pushMock.mockReset();
    dismissMock.mockReset();
    fakeCluster.contexts = [];
  });

  it("calls TriggerOrphanScan on mount for each connected context", async () => {
    triggerMock.mockResolvedValue([]);
    fakeCluster.contexts = [connectedCtx("ctx-a"), connectedCtx("ctx-b")];

    render(OrphanCleanupToast);
    await tick();
    await new Promise((r) => setTimeout(r, 10));

    expect(triggerMock).toHaveBeenCalledTimes(2);
    expect(triggerMock).toHaveBeenCalledWith("ctx-a");
    expect(triggerMock).toHaveBeenCalledWith("ctx-b");
  });

  it("shows a toast when TriggerOrphanScan returns orphans", async () => {
    triggerMock.mockResolvedValue([
      {contextName: "ctx-a", namespace: "ns", podName: "p-unique-1", pvcName: "pvc-1", createdAt: "", sessionUuid: "s"},
    ]);
    fakeCluster.contexts = [connectedCtx("ctx-a")];

    render(OrphanCleanupToast);
    await tick();
    await new Promise((r) => setTimeout(r, 10));

    expect(pushMock).toHaveBeenCalledTimes(1);
    expect(pushMock.mock.calls[0][0]).toMatch(/leftover volume browser/);
  });

  it("skips contexts that are not connected", async () => {
    triggerMock.mockResolvedValue([]);
    fakeCluster.contexts = [
      {name: "ctx-disc", cluster: "", user: "", namespace: "", status: 0},
      connectedCtx("ctx-ok"),
    ];

    render(OrphanCleanupToast);
    await tick();
    await new Promise((r) => setTimeout(r, 10));

    expect(triggerMock).toHaveBeenCalledTimes(1);
    expect(triggerMock).toHaveBeenCalledWith("ctx-ok");
  });

  it("de-dupes a push event that fires after the on-mount pull for the same orphan set", async () => {
    const orphans = [
      {contextName: "ctx-z", namespace: "ns", podName: "p-dedup-1", pvcName: "pvc-1", createdAt: "", sessionUuid: "s"},
    ];
    triggerMock.mockResolvedValue(orphans);
    fakeCluster.contexts = [connectedCtx("ctx-z")];

    // Capture the Events.On handler so we can fire it manually.
    const {Events} = await import("@wailsio/runtime");
    const handlers = new Map<string, (e: {data?: unknown}) => void>();
    (Events.On as unknown as ReturnType<typeof vi.fn>).mockImplementation(
      (name: string, cb: (e: {data?: unknown}) => void) => {
        handlers.set(name, cb);
        return () => handlers.delete(name);
      },
    );

    render(OrphanCleanupToast);
    await tick();
    await new Promise((r) => setTimeout(r, 10));
    expect(pushMock).toHaveBeenCalledTimes(1);

    // Backend emits the same orphan set shortly after — should be suppressed.
    const handler = handlers.get("volumebrowser:orphans:ctx-z");
    expect(handler).toBeTruthy();
    handler?.({data: orphans});
    await tick();
    expect(pushMock).toHaveBeenCalledTimes(1);
  });
});
