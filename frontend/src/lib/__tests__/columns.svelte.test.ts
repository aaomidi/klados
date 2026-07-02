import {describe, it, expect, vi, beforeEach} from "vitest";

const {mockGetColumnPrefs, mockSetColumnPrefs, mockGetCompactRows, mockGetDescriptor} = vi.hoisted(() => ({
  mockGetColumnPrefs: vi.fn(),
  mockSetColumnPrefs: vi.fn(),
  mockGetCompactRows: vi.fn().mockResolvedValue(false),
  mockGetDescriptor: vi.fn(),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/configservice.js", () => ({
  GetColumnPrefs: mockGetColumnPrefs,
  SetColumnPrefs: mockSetColumnPrefs,
  DeleteColumnPrefs: vi.fn(),
  GetCompactRows: mockGetCompactRows,
  SetCompactRows: vi.fn(),
}));

vi.mock("../registry/index.js", () => ({
  descriptorRegistry: {get: mockGetDescriptor},
}));

import {columnStore} from "$lib/stores/columns.svelte";

const podDescriptor = {
  group: "",
  version: "v1",
  resource: "pods",
  kind: "Pod",
  gvr: "core.v1.pods",
  columns: [
    {name: "Name", expr: "metadata.name", renderType: "text" as const},
    {name: "Namespace", expr: "metadata.namespace", renderType: "text" as const, hidden: true},
    {name: "Status", expr: "status.phase", renderType: "badge" as const},
    {name: "Age", expr: "metadata.creationTimestamp", renderType: "age" as const},
  ],
  overviewFields: [],
  detailPanels: [],
  actions: [],
};

describe("columnStore", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    columnStore.visibleColumns = [];
    columnStore.allColumns = [];
    columnStore.sortState = null;
    columnStore.compact = false;
    mockGetCompactRows.mockResolvedValue(false);
    mockGetDescriptor.mockReturnValue(podDescriptor);
  });

  it("loads descriptor defaults when no prefs exist", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);

    await columnStore.loadForGVR("core.v1.pods");

    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Status", "Age"]);
  });

  it("merges saved prefs with descriptor", async () => {
    mockGetColumnPrefs.mockResolvedValue({
      order: ["Status", "Name"],
      columns: {Name: {width: 250}, Status: {width: 120}},
      sort: null,
    });

    await columnStore.loadForGVR("core.v1.pods");

    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Status", "Name"]);
    expect(columnStore.visibleColumns.find((c) => c.name === "Name")?.width).toBe(250);
    expect(columnStore.visibleColumns.find((c) => c.name === "Status")?.width).toBe(120);
  });

  it("hidden columns appear in allColumns but not visibleColumns", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);

    await columnStore.loadForGVR("core.v1.pods");

    expect(columnStore.visibleColumns.map((c) => c.name)).not.toContain("Namespace");
    const nsEntry = columnStore.allColumns.find((e) => e.col.name === "Namespace");
    expect(nsEntry).toBeDefined();
    expect(nsEntry?.visible).toBe(false);
  });

  it("setColumnVisible hides a column", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");

    columnStore.setColumnVisible("Status", false);

    expect(columnStore.visibleColumns.map((c) => c.name)).not.toContain("Status");
    const entry = columnStore.allColumns.find((e) => e.col.name === "Status");
    expect(entry?.visible).toBe(false);
  });

  it("setColumnVisible cannot hide Name", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");

    columnStore.setColumnVisible("Name", false);

    expect(columnStore.visibleColumns.map((c) => c.name)).toContain("Name");
  });

  it("moveColumn reorders correctly", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");
    // visible order: Name, Status, Age
    columnStore.moveColumn("Status", "up");

    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Status", "Name", "Age"]);
  });

  it("moveColumn up on first column is no-op", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");

    columnStore.moveColumn("Name", "up");

    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Status", "Age"]);
  });

  it("resizeColumn updates width", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");

    columnStore.resizeColumn("Status", 200);

    expect(columnStore.visibleColumns.find((c) => c.name === "Status")?.width).toBe(200);
  });

  it("reset clears prefs and reverts to descriptor defaults", async () => {
    mockGetColumnPrefs.mockResolvedValue({
      order: ["Status", "Name"],
      columns: {},
      sort: null,
    });
    await columnStore.loadForGVR("core.v1.pods");
    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Status", "Name"]);

    columnStore.reset();

    expect(mockSetColumnPrefs).not.toHaveBeenCalledWith("core.v1.pods", null);
    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Status", "Age"]);
  });

  it("setSort updates sort state", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");

    columnStore.setSort("Age", "desc");

    expect(columnStore.sortState).toEqual({column: "Age", direction: "desc"});
  });
});

describe("ColumnStore — pinning & reorder", () => {
  beforeEach(() => {
    mockGetColumnPrefs.mockReset();
    mockSetColumnPrefs.mockReset();
    mockGetDescriptor.mockReturnValue(podDescriptor);
  });

  it("pins Name by default when no prefs are saved", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");
    expect(columnStore.isPinned("Name")).toBe(true);
    expect(columnStore.pinnedNames()).toEqual(["Name"]);
  });

  it("setPinned(true) moves the column to the front of visibleColumns", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");
    columnStore.setPinned("Status", true);
    expect(columnStore.visibleColumns[0].name).toBe("Name");
    expect(columnStore.visibleColumns[1].name).toBe("Status");
    expect(columnStore.isPinned("Status")).toBe(true);
  });

  it("setPinned(false) removes the column from the pinned set", async () => {
    mockGetColumnPrefs.mockResolvedValue({order: ["Name", "Status", "Age"], columns: {}, pinned: ["Name", "Status"]});
    await columnStore.loadForGVR("core.v1.pods");
    columnStore.setPinned("Status", false);
    expect(columnStore.isPinned("Status")).toBe(false);
    expect(columnStore.pinnedNames()).toEqual(["Name"]);
  });

  it("reorderVisible(names) replaces visibleColumns and persists", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");
    columnStore.reorderVisible(["Name", "Age", "Status"]);
    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Age", "Status"]);
    expect(mockSetColumnPrefs).toHaveBeenCalled();
  });

  it("reorderVisible ignores names not currently visible AND preserves unnamed visible columns at the end", async () => {
    mockGetColumnPrefs.mockResolvedValue(null);
    await columnStore.loadForGVR("core.v1.pods");
    const before = columnStore.visibleColumns.map((c) => c.name);
    columnStore.reorderVisible(["Name", "DoesNotExist", "Age"]);
    // Status was visible but not in the new order — must remain (appended at end)
    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Age", "Status"]);
    expect(before).not.toEqual(columnStore.visibleColumns.map((c) => c.name));
  });

  it("reorderVisible accepts a partial list (main-grid only) and preserves pinned columns", async () => {
    mockGetColumnPrefs.mockResolvedValue({order: ["Name", "Status", "Age"], columns: {}, pinned: ["Name"]});
    await columnStore.loadForGVR("core.v1.pods");
    // Simulate dndzone passing only the main columns in their new order (excluding pinned)
    columnStore.reorderVisible(["Age", "Status"]);
    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Age", "Status"]);
  });

  it("setPinned(false) moves the unpinned column to the first non-pinned position", async () => {
    mockGetColumnPrefs.mockResolvedValue({order: ["Name", "Status", "Age"], columns: {}, pinned: ["Name", "Status"]});
    await columnStore.loadForGVR("core.v1.pods");
    // Sanity: Status starts pinned at index 1, Age at index 2
    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Status", "Age"]);
    columnStore.setPinned("Status", false);
    // After unpinning, Name (pinned) at 0, Status at 1 (first unpinned), Age at 2
    expect(columnStore.visibleColumns.map((c) => c.name)).toEqual(["Name", "Status", "Age"]);
    expect(columnStore.isPinned("Status")).toBe(false);
  });
});
