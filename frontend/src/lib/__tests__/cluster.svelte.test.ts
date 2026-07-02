import {describe, it, expect, vi, beforeEach} from "vitest";
import {clusterStore, ConnectionStatus} from "$lib/stores/cluster.svelte";

// Mock the binding module
vi.mock("$api/github.com/Vilsol/klados/internal/services/clusterservice.js", () => ({
  ListContexts: vi.fn(),
  Connect: vi.fn(),
  Disconnect: vi.fn(),
  Activate: vi.fn().mockResolvedValue(undefined),
  Deactivate: vi.fn().mockResolvedValue(undefined),
  ListNamespaces: vi.fn(),
  CreateNamespace: vi.fn(),
  DeleteNamespace: vi.fn(),
  SwitchNamespace: vi.fn(),
  GetActiveNamespace: vi.fn().mockResolvedValue(""),
  GetStatus: vi.fn(),
}));
vi.mock("$api/github.com/Vilsol/klados/internal/services/appservice.js", () => ({
  SetReadOnly: vi.fn().mockResolvedValue(undefined),
  SetLastActiveContext: vi.fn().mockResolvedValue(undefined),
  LogFrontend: vi.fn().mockResolvedValue(undefined),
  GetStreamingConfig: vi.fn().mockResolvedValue({port: 0, token: ""}),
  GetClusterHealth: vi.fn(),
  SaveUIState: vi.fn(),
  GetSession: vi.fn(),
  BrowseKubeconfigFile: vi.fn(),
  BrowsePluginFile: vi.fn(),
  BrowseManifestFile: vi.fn(),
}));
vi.mock("$api/github.com/Vilsol/klados/internal/services/configservice.js", () => ({
  GetConfig: vi.fn().mockResolvedValue({readOnly: false}),
}));
vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  StartWatch: vi.fn().mockResolvedValue(undefined),
  StopWatch: vi.fn().mockResolvedValue(undefined),
}));

import {
  ListContexts,
  Connect,
  Disconnect,
  Activate,
  Deactivate,
  ListNamespaces,
} from "$api/github.com/Vilsol/klados/internal/services/clusterservice";
import {SetLastActiveContext} from "$api/github.com/Vilsol/klados/internal/services/appservice";
import {StartWatch, StopWatch} from "$api/github.com/Vilsol/klados/internal/services/resourceservice";
import {Events} from "@wailsio/runtime";

const mockedListContexts = vi.mocked(ListContexts);
const mockedConnect = vi.mocked(Connect);
const mockedDisconnect = vi.mocked(Disconnect);
const mockedActivate = vi.mocked(Activate);
const mockedDeactivate = vi.mocked(Deactivate);
const mockedSetLastActive = vi.mocked(SetLastActiveContext);
const mockedListNamespaces = vi.mocked(ListNamespaces);
const mockedStartWatch = vi.mocked(StartWatch);
const mockedStopWatch = vi.mocked(StopWatch);
const mockedEventsOn = vi.mocked(Events.On);

// Find the namespace-watch event handler registered for a context and invoke it
// with a synthetic Wails event payload.
function emitNamespaceWatch(ctx: string, type: string, name: string) {
  const eventName = `watch:${ctx}:core.v1.namespaces:`;
  const call = mockedEventsOn.mock.calls.find((c) => c[0] === eventName);
  if (!call) throw new Error(`no handler registered for ${eventName}`);
  const handler = call[1] as (e: unknown) => void;
  handler({data: {type, object: {metadata: {name}}}});
}

function invokeStatusHandler(ctx: string, status: string) {
  const call = mockedEventsOn.mock.calls.find((c) => c[0] === `status:${ctx}:connection`);
  if (!call) throw new Error(`no status handler for ${ctx}`);
  (call[1] as (e: unknown) => void)({data: status});
}

describe("clusterStore", () => {
  beforeEach(async () => {
    // Tear down any namespace watch left active by the previous test before
    // clearing mock call history.
    await clusterStore.setActiveContext(null);
    vi.clearAllMocks();
    clusterStore.contexts = [];
    clusterStore.activeContext = null;
    clusterStore.selectedNamespaces = {};
    clusterStore.namespaces = {};
    clusterStore.connectionStatus = {};
  });

  it("loadContexts populates contexts and status", async () => {
    mockedListContexts.mockResolvedValue([
      {name: "ctx1", cluster: "c1", user: "u1", namespace: "default", status: ConnectionStatus.StatusConnected},
      {name: "ctx2", cluster: "c2", user: "u2", namespace: "ns2", status: ConnectionStatus.StatusDisconnected},
    ] as never);

    await clusterStore.loadContexts();

    expect(clusterStore.contexts).toHaveLength(2);
    expect(clusterStore.connectionStatus.ctx1).toBe("connected");
    expect(clusterStore.connectionStatus.ctx2).toBe("disconnected");
  });

  it("connect updates status without auto-setting active context", async () => {
    mockedConnect.mockResolvedValue(undefined);
    mockedListNamespaces.mockResolvedValue(["default", "kube-system"] as never);

    await clusterStore.connect("ctx1");

    expect(mockedConnect).toHaveBeenCalledWith("ctx1");
    expect(clusterStore.connectionStatus.ctx1).toBe("connected");
    expect(clusterStore.activeContext).toBeNull();
    expect(mockedActivate).not.toHaveBeenCalled();
    expect(clusterStore.getNamespaces("ctx1")).toEqual(["default", "kube-system"]);
  });

  it("connect does not override activeContext when already set", async () => {
    clusterStore.activeContext = "ctx1";
    mockedConnect.mockResolvedValue(undefined);
    mockedListNamespaces.mockResolvedValue([] as never);

    await clusterStore.connect("ctx2");

    expect(clusterStore.activeContext).toBe("ctx1");
    expect(clusterStore.connectionStatus.ctx2).toBe("connected");
  });

  it("setActiveContext activates new and deactivates previous", async () => {
    clusterStore.activeContext = "ctx1";

    await clusterStore.setActiveContext("ctx2");

    expect(mockedDeactivate).toHaveBeenCalledWith("ctx1");
    expect(mockedActivate).toHaveBeenCalledWith("ctx2");
    expect(mockedSetLastActive).toHaveBeenCalledWith("ctx2");
    expect(clusterStore.activeContext).toBe("ctx2");
  });

  it("setActiveContext is no-op when same context", async () => {
    clusterStore.activeContext = "ctx1";

    await clusterStore.setActiveContext("ctx1");

    expect(mockedActivate).not.toHaveBeenCalled();
    expect(mockedDeactivate).not.toHaveBeenCalled();
  });

  it("setActiveContext(null) deactivates current and persists empty", async () => {
    clusterStore.activeContext = "ctx1";

    await clusterStore.setActiveContext(null);

    expect(mockedDeactivate).toHaveBeenCalledWith("ctx1");
    expect(mockedActivate).not.toHaveBeenCalled();
    expect(mockedSetLastActive).toHaveBeenCalledWith("");
    expect(clusterStore.activeContext).toBeNull();
  });

  it("connect sets error status on failure", async () => {
    mockedConnect.mockRejectedValue(new Error("fail"));

    await clusterStore.connect("ctx1");

    expect(clusterStore.connectionStatus.ctx1).toBe("error");
  });

  it("disconnect clears active context", async () => {
    clusterStore.activeContext = "ctx1";
    clusterStore.namespaces = {ctx1: ["default"]};
    mockedDisconnect.mockResolvedValue(undefined);

    await clusterStore.disconnect("ctx1");

    expect(mockedDisconnect).toHaveBeenCalledWith("ctx1");
    expect(clusterStore.connectionStatus.ctx1).toBe("disconnected");
    expect(clusterStore.activeContext).toBeNull();
    expect(clusterStore.getNamespaces("ctx1")).toEqual([]);
  });

  it("disconnect preserves other connected context as active", async () => {
    clusterStore.activeContext = "ctx1";
    clusterStore.connectionStatus = {ctx1: "connected", ctx2: "connected"};
    mockedDisconnect.mockResolvedValue(undefined);

    await clusterStore.disconnect("ctx1");

    expect(clusterStore.activeContext).toBe("ctx2");
  });

  it("setActiveContext starts a namespace watch for the new context", async () => {
    mockedListNamespaces.mockResolvedValue(["default"] as never);

    await clusterStore.setActiveContext("ctx1");

    expect(mockedStartWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "", "");
  });

  it("namespace ADDED watch event adds to the dropdown list", async () => {
    mockedListNamespaces.mockResolvedValue(["default"] as never);
    await clusterStore.setActiveContext("ctx1");

    emitNamespaceWatch("ctx1", "ADDED", "new-ns");

    expect(clusterStore.getNamespaces("ctx1")).toContain("new-ns");
  });

  it("namespace DELETED watch event removes from the dropdown list", async () => {
    mockedListNamespaces.mockResolvedValue(["default", "doomed"] as never);
    await clusterStore.setActiveContext("ctx1");

    emitNamespaceWatch("ctx1", "DELETED", "doomed");

    expect(clusterStore.getNamespaces("ctx1")).not.toContain("doomed");
  });

  it("switching active context stops the previous namespace watch", async () => {
    mockedListNamespaces.mockResolvedValue([] as never);
    await clusterStore.setActiveContext("ctx1");
    await clusterStore.setActiveContext("ctx2");

    expect(mockedStopWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "");
    expect(mockedStartWatch).toHaveBeenCalledWith("ctx2", "core.v1.namespaces", "", "");
  });

  it("setNamespaces updates selected namespaces per context", async () => {
    await clusterStore.setNamespaces("ctx1", ["kube-system"]);

    expect(clusterStore.getSelectedNamespaces("ctx1")).toEqual(["kube-system"]);
    expect(clusterStore.getSelectedNamespaces("ctx2")).toEqual([]);
  });

  it("reconnect of the active context restarts the namespace watch", async () => {
    mockedListContexts.mockResolvedValue([
      {name: "ctx1", cluster: "c1", user: "u1", namespace: "default", status: ConnectionStatus.StatusConnected},
    ] as never);
    mockedListNamespaces.mockResolvedValue(["default"] as never);
    await clusterStore.loadContexts(); // registers status handler + auto-restores ctx1

    mockedStartWatch.mockClear();
    mockedStopWatch.mockClear();
    invokeStatusHandler("ctx1", "error");
    invokeStatusHandler("ctx1", "connected");

    await vi.waitFor(() => {
      expect(mockedStopWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "");
      expect(mockedStartWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "", "");
    });
  });

  it("namespace watch :resync reloads the list and re-watches", async () => {
    mockedListNamespaces.mockResolvedValue(["default"] as never);
    await clusterStore.setActiveContext("ctx1");
    mockedListNamespaces.mockClear();
    mockedStartWatch.mockClear();

    const call = mockedEventsOn.mock.calls.find((c) => c[0] === "watch:ctx1:core.v1.namespaces::resync");
    expect(call).toBeDefined();
    (call![1] as () => void)();

    await vi.waitFor(() => {
      expect(mockedListNamespaces).toHaveBeenCalledWith("ctx1");
      expect(mockedStartWatch).toHaveBeenCalledWith("ctx1", "core.v1.namespaces", "", "");
    });
  });
});
