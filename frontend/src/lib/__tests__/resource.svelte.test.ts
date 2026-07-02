import {describe, it, expect, vi, beforeEach} from "vitest";

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  ListResourcesWithVersion: vi.fn().mockResolvedValue({items: [], resourceVersion: "1"}),
  StartWatch: vi.fn().mockResolvedValue(undefined),
  StopWatch: vi.fn().mockResolvedValue(undefined),
}));

import {ListResourcesWithVersion} from "$api/github.com/Vilsol/klados/internal/services/resourceservice";
import {Events} from "@wailsio/runtime";
import {ResourceStore} from "$lib/stores/resource.svelte";

const mockedList = vi.mocked(ListResourcesWithVersion);
const mockedEventsOn = vi.mocked(Events.On);

function invokeHandler(eventName: string, payload: unknown) {
  const call = mockedEventsOn.mock.calls.find((c) => c[0] === eventName);
  if (!call) throw new Error(`no handler for ${eventName}`);
  (call[1] as (e: unknown) => void)({data: payload});
}

describe("ResourceStore reconnect handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedList.mockResolvedValue({items: [], resourceVersion: "1"} as never);
  });

  it("re-lists when the connection recovers from error", async () => {
    const store = new ResourceStore();
    await store.start("ctx1", "core.v1.pods", "default");
    expect(mockedList).toHaveBeenCalledTimes(1);

    invokeHandler("status:ctx1:connection", "error");
    invokeHandler("status:ctx1:connection", "connected");
    await vi.waitFor(() => expect(mockedList).toHaveBeenCalledTimes(2));

    store.stop();
  });

  it("re-lists on kubeconfig re-import (disconnected → connecting → connected)", async () => {
    const store = new ResourceStore();
    await store.start("ctx1", "core.v1.pods", "default");
    expect(mockedList).toHaveBeenCalledTimes(1);

    invokeHandler("status:ctx1:connection", "disconnected");
    invokeHandler("status:ctx1:connection", "connecting");
    invokeHandler("status:ctx1:connection", "connected");
    await vi.waitFor(() => expect(mockedList).toHaveBeenCalledTimes(2));

    store.stop();
  });

  it("does not re-list on a connected event without a preceding failure", async () => {
    const store = new ResourceStore();
    await store.start("ctx1", "core.v1.pods", "default");

    invokeHandler("status:ctx1:connection", "connected");
    await new Promise((r) => setTimeout(r, 20));
    expect(mockedList).toHaveBeenCalledTimes(1);

    store.stop();
  });
});
