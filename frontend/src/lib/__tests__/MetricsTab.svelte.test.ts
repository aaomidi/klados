import {describe, it, expect, vi, beforeEach} from "vitest";
import {render, waitFor} from "@testing-library/svelte";

vi.mock("uplot", () => {
  class UPlot {
    setData = vi.fn();
    destroy = vi.fn();
    setSeries = vi.fn();
    setSize = vi.fn();
    setScale = vi.fn();
    over = {addEventListener: vi.fn()};
    cursor = {idx: null, left: 0, top: 0};
    scales = {x: {min: 0, max: 0}};
    series = [];
    data: unknown[] = [];
    constructor(_opts: unknown, _data: unknown, el: HTMLElement) {
      this.data = _data as unknown[];
      const canvas = document.createElement("canvas");
      el?.appendChild(canvas);
    }
  }
  return {default: UPlot};
});
vi.mock("uplot/dist/uPlot.min.css", () => ({}));

const mockGetCapabilities = vi.hoisted(() => vi.fn());
const mockGetResourceMetrics = vi.hoisted(() => vi.fn());

vi.mock("$api/github.com/Vilsol/klados/internal/services/metricsservice.js", () => ({
  GetCapabilities: mockGetCapabilities,
  GetResourceMetrics: mockGetResourceMetrics,
}));

const makeCapability = (opts: {metricsServer?: boolean; prometheus?: boolean} = {}) => ({
  hasMetricsServer: opts.metricsServer ?? true,
  hasPrometheus: opts.prometheus ?? false,
});

const makeResponse = () => ({
  metrics: [
    {
      name: "CPU Usage",
      unit: "cores",
      series: [{labels: {container: "nginx"}, points: [{t: 1000, v: 0.1}]}],
    },
    {
      name: "Memory Usage",
      unit: "bytes",
      series: [{labels: {container: "nginx"}, points: [{t: 1000, v: 134_217_728}]}],
    },
  ],
  thresholds: [],
  annotations: [],
});

beforeEach(() => {
  vi.clearAllMocks();
  mockGetCapabilities.mockResolvedValue(makeCapability());
  mockGetResourceMetrics.mockResolvedValue(makeResponse());
});

const defaultProps = {
  obj: {},
  ctxName: "test-ctx",
  gvr: "core.v1.pods",
  namespace: "default",
  name: "nginx-abc",
};

describe("MetricsTab", () => {
  it("calls GetCapabilities on mount", async () => {
    const MetricsTab = (await import("$lib/components/charts/MetricsTab.svelte")).default;
    render(MetricsTab, {props: defaultProps});
    await waitFor(() => expect(mockGetCapabilities).toHaveBeenCalledWith("test-ctx"));
  });

  it("calls GetResourceMetrics on mount when capabilities are available", async () => {
    const MetricsTab = (await import("$lib/components/charts/MetricsTab.svelte")).default;
    render(MetricsTab, {props: defaultProps});
    await waitFor(() => expect(mockGetResourceMetrics).toHaveBeenCalledWith("test-ctx", "core.v1.pods", "default", "nginx-abc", 15));
  });

  it("sets up polling interval and clears it on unmount", async () => {
    const setIntervalSpy = vi.spyOn(globalThis, "setInterval");
    const clearIntervalSpy = vi.spyOn(globalThis, "clearInterval");

    const MetricsTab = (await import("$lib/components/charts/MetricsTab.svelte")).default;
    const {unmount} = render(MetricsTab, {props: defaultProps});

    await waitFor(() => expect(setIntervalSpy).toHaveBeenCalled());
    unmount();
    expect(clearIntervalSpy).toHaveBeenCalled();
  });

  it("renders nothing when no metric sources are available", async () => {
    mockGetCapabilities.mockResolvedValue(makeCapability({metricsServer: false, prometheus: false}));
    const MetricsTab = (await import("$lib/components/charts/MetricsTab.svelte")).default;
    const {container} = render(MetricsTab, {props: defaultProps});

    // Wait for capability fetch to complete
    await waitFor(() => expect(mockGetCapabilities).toHaveBeenCalled());

    // No charts or content should be rendered
    expect(container.querySelector("canvas")).toBeNull();
  });
});
