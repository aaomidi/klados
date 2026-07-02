import {describe, it, expect, vi, beforeEach} from "vitest";

const spawnMock = vi.fn();
const replaceMock = vi.fn();
const attachTabMock = vi.fn();

vi.mock("$api/github.com/Vilsol/klados/internal/services/volumebrowserservice.js", () => ({
  Spawn: (...args: unknown[]) => spawnMock(...args),
  Replace: (...args: unknown[]) => replaceMock(...args),
  AttachTab: (...args: unknown[]) => attachTabMock(...args),
}));

const notifyError = vi.fn();
const notifyPush = vi.fn();
vi.mock("$lib/stores/notification.svelte", () => ({
  notificationStore: {
    error: (...a: unknown[]) => notifyError(...a),
    push: (...a: unknown[]) => notifyPush(...a),
  },
}));

const addTabMock = vi.fn().mockReturnValue("tab-1");
vi.mock("$lib/stores/bottom-panel.svelte", () => ({
  bottomPanelStore: {
    addTab: (...a: unknown[]) => addTabMock(...a),
  },
}));

// Preferences mock — reactive $state not needed here
vi.mock("$lib/stores/preferences.svelte", () => ({
  preferencesStore: {
    prefs: {
      volumeBrowser: {
        image: "alpine:edge",
        mountPath: "/mnt/volume",
        orphanCleanupOnStartup: "prompt",
      },
    },
  },
}));

import {volumeBrowserStore} from "$lib/stores/volumeBrowser.svelte";

describe("volumeBrowserStore.spawn", () => {
  beforeEach(() => {
    spawnMock.mockReset();
    replaceMock.mockReset();
    attachTabMock.mockReset().mockResolvedValue(undefined);
    addTabMock.mockClear();
    notifyError.mockClear();
    notifyPush.mockClear();
    volumeBrowserStore.dialog = null;
    volumeBrowserStore.collision = null;
  });

  it("happy path: calls Spawn, adds terminal-pending tab, attaches tab", async () => {
    spawnMock.mockResolvedValue({id: "sess-1", namespace: "ns1", podName: "pvc-browser-abc"});
    await volumeBrowserStore.spawn("ctx", "ns1", "data-pvc");
    expect(spawnMock).toHaveBeenCalledTimes(1);
    const req = spawnMock.mock.calls[0][0];
    expect(req.contextName).toBe("ctx");
    expect(req.namespace).toBe("ns1");
    expect(req.pvcName).toBe("data-pvc");
    expect(addTabMock).toHaveBeenCalledTimes(1);
    const tab = addTabMock.mock.calls[0][0];
    expect(tab.kind).toBe("terminal-pending");
    expect(tab.managedId).toBe("sess-1");
    expect(attachTabMock).toHaveBeenCalledWith("sess-1", "tab-1");
    expect(notifyError).not.toHaveBeenCalled();
  });

  it("collision → Replace path", async () => {
    spawnMock.mockRejectedValueOnce({existingPodName: "pvc-browser-old", existingId: "old-id", error: "collision"});
    replaceMock.mockResolvedValue({id: "new-id", namespace: "ns1", podName: "pvc-browser-new"});

    const p = volumeBrowserStore.spawn("ctx", "ns1", "data-pvc");
    // wait for collision dialog to open
    await Promise.resolve();
    await Promise.resolve();
    expect(volumeBrowserStore.collision).not.toBeNull();
    volumeBrowserStore.collision?.resolve("replace");
    await p;

    expect(replaceMock).toHaveBeenCalledTimes(1);
    expect(replaceMock.mock.calls[0][0]).toBe("old-id");
    expect(addTabMock).toHaveBeenCalledTimes(1);
    expect(addTabMock.mock.calls[0][0].managedId).toBe("new-id");
  });

  it("collision → cancel path", async () => {
    spawnMock.mockRejectedValueOnce({existingPodName: "p", existingId: "id", error: "collision"});
    const p = volumeBrowserStore.spawn("ctx", "ns1", "data-pvc");
    await Promise.resolve();
    await Promise.resolve();
    volumeBrowserStore.collision?.resolve("cancel");
    await p;
    expect(replaceMock).not.toHaveBeenCalled();
    expect(addTabMock).not.toHaveBeenCalled();
  });

  it("collision → attach path uses existing pod without calling Replace", async () => {
    spawnMock.mockRejectedValueOnce({existingPodName: "existing-pod", existingId: "ex-id", error: "collision"});
    const p = volumeBrowserStore.spawn("ctx", "ns1", "data-pvc");
    await Promise.resolve();
    await Promise.resolve();
    volumeBrowserStore.collision?.resolve("attach");
    await p;
    expect(replaceMock).not.toHaveBeenCalled();
    expect(addTabMock).toHaveBeenCalledTimes(1);
    expect(addTabMock.mock.calls[0][0].managedId).toBe("ex-id");
    expect(addTabMock.mock.calls[0][0].name).toBe("existing-pod");
    expect(attachTabMock).toHaveBeenCalledWith("ex-id", "tab-1");
  });

  it("generic error → toast + no tab", async () => {
    spawnMock.mockRejectedValueOnce(new Error("boom"));
    await volumeBrowserStore.spawn("ctx", "ns1", "data-pvc");
    expect(notifyError).toHaveBeenCalled();
    expect(addTabMock).not.toHaveBeenCalled();
  });

  it("cancelled dialog (forceDialog) does not call Spawn", async () => {
    const p = volumeBrowserStore.spawn("ctx", "ns1", "data-pvc", {forceDialog: true});
    await Promise.resolve();
    expect(volumeBrowserStore.dialog).not.toBeNull();
    volumeBrowserStore.dialog?.resolve(null);
    await p;
    expect(spawnMock).not.toHaveBeenCalled();
  });
});
