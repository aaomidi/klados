import {describe, it, expect, vi, beforeEach} from "vitest";
import {render} from "@testing-library/svelte";
import PodDiagnosisBanner from "$lib/components/panels/PodDiagnosisBanner.svelte";

vi.mock("../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetEvents: vi.fn(() => Promise.resolve([])),
  StartWatch: vi.fn(() => Promise.resolve()),
  StopWatch: vi.fn(() => Promise.resolve()),
}));

function pod(status: Record<string, unknown>) {
  return {metadata: {uid: "u1", namespace: "default"}, status};
}

describe("PodDiagnosisBanner", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders nothing for a healthy pod", () => {
    const {container} = render(PodDiagnosisBanner, {
      props: {
        obj: pod({phase: "Running", containerStatuses: [{name: "app", ready: true, state: {running: {}}}]}),
        ctxName: "c",
        namespace: "default",
        uid: "u1",
        setActivePanel: () => {},
      },
    });
    expect(container.textContent?.trim()).toBe("");
  });

  it("shows eviction reason and message", () => {
    const {getByText} = render(PodDiagnosisBanner, {
      props: {
        obj: pod({phase: "Failed", reason: "Evicted", message: "The node was low on resource: memory."}),
        ctxName: "c",
        namespace: "default",
        uid: "u1",
        setActivePanel: () => {},
      },
    });
    expect(getByText("Evicted")).toBeTruthy();
    expect(getByText(/low on resource: memory/)).toBeTruthy();
  });

  it("shows unhealthy container summary with a jump link", () => {
    const setActivePanel = vi.fn();
    const {getByText} = render(PodDiagnosisBanner, {
      props: {
        obj: pod({
          phase: "Running",
          containerStatuses: [
            {name: "app", ready: false, state: {waiting: {reason: "CrashLoopBackOff"}}},
            {name: "side", ready: true, state: {running: {}}},
          ],
        }),
        ctxName: "c",
        namespace: "default",
        uid: "u1",
        setActivePanel,
      },
    });
    const link = getByText(/1 of 2 containers unhealthy/);
    expect(link).toBeTruthy();
    (link as HTMLElement).click();
    expect(setActivePanel).toHaveBeenCalledWith("overview");
  });
});
