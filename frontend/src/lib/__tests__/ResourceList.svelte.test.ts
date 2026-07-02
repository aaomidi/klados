import {describe, it, expect, vi, beforeEach} from "vitest";
import {render} from "@testing-library/svelte";
import type {ColumnDef} from "$lib/registry/index";

const {mockSetNamespaces, mockSetSort, mockResizeColumn, mockAutoFitColumn, mockVisibleColumns, mockSortState, mockCompact} = vi.hoisted(
  () => ({
    mockSetNamespaces: vi.fn(),
    mockSetSort: vi.fn(),
    mockResizeColumn: vi.fn(),
    mockAutoFitColumn: vi.fn(),
    mockVisibleColumns: {value: [] as ColumnDef[]},
    mockSortState: {value: null as {column: string; direction: "asc" | "desc"} | null},
    mockCompact: {value: false},
  }),
);

vi.mock("$lib/stores/columns.svelte", () => ({
  columnStore: {
    get visibleColumns() {
      return mockVisibleColumns.value;
    },
    get allColumns() {
      return mockVisibleColumns.value.map((c) => ({col: c, visible: true}));
    },
    get sortState() {
      return mockSortState.value;
    },
    get compact() {
      return mockCompact.value;
    },
    setSort: mockSetSort,
    resizeColumn: mockResizeColumn,
    autoFitColumn: mockAutoFitColumn,
    setColumnVisible: vi.fn(),
    reorderVisible: vi.fn(),
    setPinned: vi.fn(),
    isPinned: vi.fn().mockReturnValue(false),
    pinnedNames: vi.fn().mockReturnValue([]),
    reset: vi.fn(),
    setCompact: vi.fn(),
  },
}));

vi.mock("$lib/stores/cluster.svelte", () => ({
  clusterStore: {
    setNamespaces: mockSetNamespaces,
    canMutate: vi.fn().mockReturnValue(false),
  },
}));

vi.mock("$lib/stores/selection.svelte", () => ({
  selectionStore: {
    selectedKeys: new Set(),
    selectedGVR: "",
    count: 0,
    notVisibleCount: 0,
    isSelected: vi.fn().mockReturnValue(false),
    toggle: vi.fn(),
    selectRange: vi.fn(),
    selectAll: vi.fn(),
    deselectAll: vi.fn(),
    setVisibleKeys: vi.fn(),
    setGVR: vi.fn(),
    items: vi.fn().mockReturnValue([]),
  },
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  DeleteResource: vi.fn(),
  ListResources: vi.fn().mockResolvedValue([]),
}));

vi.mock("$lib/plugins/slots.svelte.js", () => ({
  slotRegistry: {
    getListColumns: vi.fn().mockReturnValue([]),
    getContextMenuItems: vi.fn().mockReturnValue([]),
  },
}));

vi.mock("$lib/stores/streaming.svelte.js", () => ({
  streamingStore: {config: null},
}));

vi.mock("$lib/plugins/loader.js", () => ({
  loadPluginComponent: vi.fn().mockResolvedValue(null),
}));

vi.mock("./charts/Sparkline.svelte", () => ({default: vi.fn()}));

vi.mock("@klados/ui", () => ({
  ConfirmDialog: vi.fn(),
}));

// Mock virtualizer to return all items (jsdom has no scroll/layout)
vi.mock("@tanstack/svelte-virtual", () => ({
  createVirtualizer: ({count: initialCount}: {count: number}) => {
    let currentCount = initialCount;
    let emit: ((v: unknown) => void) | null = null;
    const buildValue = () => ({
      getTotalSize: () => currentCount * 36,
      getVirtualItems: () =>
        Array.from({length: currentCount}, (_, i) => ({index: i, start: i * 36, size: 36})),
      setOptions: (opts: {count?: number}) => {
        if (opts.count !== undefined && opts.count !== currentCount) {
          currentCount = opts.count;
          emit?.(buildValue());
        }
      },
    });
    return {
      subscribe: (fn: (v: unknown) => void) => {
        emit = fn;
        fn(buildValue());
        return () => {
          emit = null;
        };
      },
    };
  },
}));

import ResourceList from "$lib/components/ResourceList.svelte";

const textCol: ColumnDef = {name: "Name", expr: "metadata.name", renderType: "text"};
const ageCol: ColumnDef = {name: "Age", expr: "metadata.creationTimestamp", renderType: "age"};
const _nsCol: ColumnDef = {name: "Namespace", expr: "metadata.namespace", renderType: "text"};

const testItem = {
  metadata: {name: "my-pod", namespace: "default", creationTimestamp: "2024-01-01T00:00:00Z"},
  spec: {},
  status: {},
};

describe("ResourceList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockVisibleColumns.value = [textCol, ageCol];
    mockSortState.value = null;
    mockCompact.value = false;
  });

  it("first column has no sticky or shadow classes", () => {
    const {container} = render(ResourceList, {
      props: {
        items: [],
        contextName: "test-ctx",
        gvr: "core.v1.pods",
      },
    });

    const headerButtons = container.querySelectorAll('.grid [role="button"]');
    const first = headerButtons[0] as HTMLElement;
    expect(first.className).not.toContain("sticky");
    expect(first.className).not.toContain("shadow");
  });

  it("cell alignment matches render type", () => {
    mockVisibleColumns.value = [textCol, ageCol];

    const {container} = render(ResourceList, {
      props: {
        items: [testItem],
        contextName: "test-ctx",
        gvr: "core.v1.pods",
      },
    });

    const bodyCells = container.querySelectorAll("[data-col]");
    const nameCell = Array.from(bodyCells).find((el) => el.getAttribute("data-col") === "Name") as HTMLElement;
    const ageCell = Array.from(bodyCells).find((el) => el.getAttribute("data-col") === "Age") as HTMLElement;

    expect(nameCell?.className).toContain("text-left");
    expect(ageCell?.className).toContain("text-right");
  });

  it("cell has title attribute", () => {
    mockVisibleColumns.value = [textCol, ageCol];

    const {container} = render(ResourceList, {
      props: {
        items: [testItem],
        contextName: "test-ctx",
        gvr: "core.v1.pods",
      },
    });

    const spans = container.querySelectorAll("[data-col] span");
    expect(spans.length).toBeGreaterThan(0);
    for (const span of spans) {
      expect(span.hasAttribute("title")).toBe(true);
    }
  });

  it("sorts numeric values numerically", () => {
    const restartsCol: ColumnDef = {name: "Restarts", expr: "status.restartCount", renderType: "text", width: 80};
    mockVisibleColumns.value = [textCol, restartsCol];
    mockSortState.value = {column: "Restarts", direction: "asc"};

    const items = [
      {metadata: {name: "pod-a"}, spec: {}, status: {restartCount: 10}},
      {metadata: {name: "pod-b"}, spec: {}, status: {restartCount: 2}},
      {metadata: {name: "pod-c"}, spec: {}, status: {restartCount: 1}},
    ];

    const {container} = render(ResourceList, {
      props: {items, contextName: "test-ctx", gvr: "core.v1.pods"},
    });

    const nameCells = container.querySelectorAll('[data-col="Name"] span');
    const names = Array.from(nameCells).map((el) => el.textContent);
    expect(names).toEqual(["pod-c", "pod-b", "pod-a"]);
  });
});
