import {describe, it, expect, vi, beforeEach} from "vitest";
import {render, screen, fireEvent, waitFor} from "@testing-library/svelte";

// Hoisted mock state so the resource store factory returns a controllable store
const {mockItems, mockLoading} = vi.hoisted(() => ({
  mockItems: {value: [] as unknown[]},
  mockLoading: {value: false},
}));

vi.mock("$lib/stores/resource.svelte", () => ({
  createResourceStore: () => ({
    get items() { return mockItems.value; },
    get loading() { return mockLoading.value; },
    get error() { return null; },
    get lastLoadMs() { return null; },
    start: vi.fn(),
    stop: vi.fn(),
  }),
}));

vi.mock("$lib/stores/columns.svelte", () => ({
  columnStore: {
    get visibleColumns() {
      return [
        {name: "Type"},
        {name: "Reason"},
        {name: "Object"},
        {name: "Message"},
        {name: "Count"},
        {name: "Last seen"},
      ];
    },
    get allColumns() { return []; },
    get sortState() { return null; },
    get compact() { return false; },
    loadForGVR: vi.fn(),
    setSort: vi.fn(),
    resizeColumn: vi.fn(),
    setColumnVisible: vi.fn(),
    moveColumn: vi.fn(),
    reset: vi.fn(),
    setCompact: vi.fn(),
  },
}));

vi.mock("$lib/stores/cluster.svelte", () => ({
  clusterStore: {
    setActiveContext: vi.fn(),
    getSelectedNamespaces: vi.fn().mockReturnValue([]),
    resolveOwnerGVR: vi.fn().mockReturnValue(null),
  },
}));

vi.mock("$lib/stores/notification.svelte", () => ({
  notificationStore: {push: vi.fn()},
}));

vi.mock("$lib/registry/index", () => ({
  descriptorRegistry: {get: vi.fn().mockReturnValue(null)},
}));

vi.mock("$lib/registry/loaded.svelte", () => ({
  registryLoaded: vi.fn().mockReturnValue(false),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetResource: vi.fn().mockResolvedValue(null),
  ListResources: vi.fn().mockResolvedValue([]),
  StartWatch: vi.fn().mockResolvedValue(undefined),
  StopWatch: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/configservice.js", () => ({
  GetColumnPrefs: vi.fn().mockResolvedValue(null),
  GetCompactRows: vi.fn().mockResolvedValue(false),
  SetColumnPrefs: vi.fn().mockResolvedValue(undefined),
  SetCompactRows: vi.fn().mockResolvedValue(undefined),
  DeleteColumnPrefs: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/config/models.js", () => ({
  GVRColumnPrefs: vi.fn(),
  ColumnSettings: vi.fn(),
  SortPrefs: vi.fn(),
}));

vi.mock("@tanstack/svelte-virtual", () => ({
  createVirtualizer: ({count}: {count: number}) => ({
    subscribe: (fn: (v: unknown) => void) => {
      fn({
        getTotalSize: () => count * 36,
        getVirtualItems: () =>
          Array.from({length: count}, (_, i) => ({index: i, start: i * 36, size: 36})),
      });
      return () => {};
    },
  }),
}));

vi.mock("@klados/ui", () => ({
  DetailDrawer: vi.fn(),
  Combobox: vi.fn(),
}));

vi.mock("$lib/event/EventTypeBadge.svelte", () => ({default: vi.fn()}));
vi.mock("$lib/components/events/EventSeverityTimeline.svelte", () => ({default: vi.fn()}));
vi.mock("$lib/components/ResourceDetail.svelte", () => ({default: vi.fn()}));
vi.mock("$lib/components/events/EventDetailPanel.svelte", () => ({default: vi.fn()}));

import EventStreamPage from "../../routes/EventStreamPage.svelte";

const warningEvent = {
  type: "Warning",
  reason: "BackOff",
  message: "Back-off restarting failed container",
  count: 5,
  lastTimestamp: new Date(Date.now() - 30_000).toISOString(),
  metadata: {name: "evt-1", namespace: "default", uid: "uid-1"},
  involvedObject: {kind: "Pod", name: "my-pod", namespace: "default", apiVersion: "v1", uid: "pod-uid-1"},
};

const normalEvent = {
  type: "Normal",
  reason: "Pulled",
  message: "Successfully pulled image",
  count: 1,
  lastTimestamp: new Date(Date.now() - 60_000).toISOString(),
  metadata: {name: "evt-2", namespace: "default", uid: "uid-2"},
  involvedObject: {kind: "Pod", name: "my-pod", namespace: "default", apiVersion: "v1", uid: "pod-uid-1"},
};

const normalEvent2 = {
  type: "Normal",
  reason: "Pulled",
  message: "Successfully pulled image",
  count: 2,
  lastTimestamp: new Date(Date.now() - 90_000).toISOString(),
  metadata: {name: "evt-3", namespace: "default", uid: "uid-3"},
  involvedObject: {kind: "Pod", name: "my-pod", namespace: "default", apiVersion: "v1", uid: "pod-uid-1"},
};

describe("EventStreamPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockItems.value = [];
    mockLoading.value = false;
  });

  it("renders warning rows and hides them when Warning toggle is clicked", async () => {
    mockItems.value = [warningEvent, normalEvent];

    render(EventStreamPage, {props: {params: {ctx: "my-cluster"}}});

    // Both rows visible initially (DataTable virtualizer renders them)
    await waitFor(() => expect(screen.getByText("BackOff")).toBeTruthy());
    expect(screen.getByText("Pulled")).toBeTruthy();

    // Click Warning toggle button
    const warningBtn = screen.getByRole("button", {name: /Warning/i});
    await fireEvent.click(warningBtn);

    // Warning row should no longer be rendered
    await waitFor(() => expect(screen.queryByText("BackOff")).toBeNull());
    // Normal row stays
    expect(screen.getByText("Pulled")).toBeTruthy();
  });

  it("groups identical reason+object rows when Group toggle is clicked", async () => {
    // Two Normal/Pulled events for same pod — should collapse to one grouped row
    mockItems.value = [normalEvent, normalEvent2];

    render(EventStreamPage, {props: {params: {ctx: "my-cluster"}}});

    await waitFor(() => {
      const cells = screen.getAllByText("Pulled");
      expect(cells.length).toBe(2);
    });

    const groupBtn = screen.getByTestId("grouped-toggle");
    await fireEvent.click(groupBtn);

    // After grouping, only one "Pulled" row should exist
    await waitFor(() => {
      const cells = screen.getAllByText("Pulled");
      expect(cells.length).toBe(1);
    });
  });
});
