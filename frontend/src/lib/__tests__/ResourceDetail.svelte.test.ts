import {describe, it, expect, vi} from "vitest";
import {render, screen, fireEvent} from "@testing-library/svelte";
import type {DescriptorDef} from "$lib/registry/index";

// Mock all service bindings used by child components
vi.mock("../../../bindings/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  GetEvents: vi.fn().mockResolvedValue([]),
  StartWatch: vi.fn().mockResolvedValue(undefined),
  StopWatch: vi.fn().mockResolvedValue(undefined),
  UpdateResource: vi.fn().mockResolvedValue({}),
  DeleteResource: vi.fn().mockResolvedValue(undefined),
  ForceDeleteResource: vi.fn().mockResolvedValue(undefined),
  ScaleResource: vi.fn().mockResolvedValue(undefined),
  RestartResource: vi.fn().mockResolvedValue(undefined),
  PauseRollout: vi.fn().mockResolvedValue(undefined),
  ResumeRollout: vi.fn().mockResolvedValue(undefined),
  RollbackToRevision: vi.fn().mockResolvedValue(undefined),
  DeleteJobCascade: vi.fn().mockResolvedValue(undefined),
  DeleteJobOrphan: vi.fn().mockResolvedValue(undefined),
  TriggerCronJob: vi.fn().mockResolvedValue(undefined),
  SuspendCronJob: vi.fn().mockResolvedValue(undefined),
  ResumeCronJob: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("../../../bindings/github.com/Vilsol/klados/internal/services/drainservice.js", () => ({
  StartDrain: vi.fn().mockResolvedValue(undefined),
  CancelDrain: vi.fn().mockResolvedValue(undefined),
  CordonNode: vi.fn().mockResolvedValue(undefined),
  UncordonNode: vi.fn().mockResolvedValue(undefined),
  IsActive: vi.fn().mockResolvedValue(false),
  ListActive: vi.fn().mockResolvedValue([]),
}));

vi.mock("../../../bindings/github.com/Vilsol/klados/internal/services/schemaservice.js", () => ({
  GetSchema: vi.fn().mockResolvedValue({}),
}));

// CodeMirror DOM operations don't work in jsdom — skip by mocking YAMLEditor
// ConfirmDialog and Tooltip are used by ActionsToolbar (child of ResourceDetail)
vi.mock("@klados/ui", () => ({
  YAMLEditor: vi.fn(),
  ConfirmDialog: vi.fn(),
  Tooltip: vi.fn(),
  SectionHeader: vi.fn(),
  KeyValueBadge: vi.fn(),
  EmptyState: vi.fn(),
  StatusBadge: vi.fn(),
  KeyValuePairEditor: vi.fn(),
  DataTable: vi.fn(),
  CopyableValue: vi.fn(),
}));

import ResourceDetail from "$lib/components/ResourceDetail.svelte";

const deployDescriptor: DescriptorDef = {
  group: "apps",
  version: "v1",
  resource: "deployments",
  kind: "",
  gvr: "apps.v1.deployments",
  columns: [],
  overviewFields: [{label: "Namespace", expr: "metadata.namespace", renderType: "text"}],
  detailPanels: ["overview", "events"],
  actions: [
    {name: "scale", label: "Scale"},
    {name: "restart", label: "Restart"},
    {name: "delete", label: "Delete"},
  ],
};

const obj = {
  metadata: {name: "my-deploy", namespace: "default", uid: "uid-123", creationTimestamp: new Date().toISOString()},
  spec: {replicas: 2, strategy: {type: "RollingUpdate"}},
  status: {replicas: 2, readyReplicas: 2},
};

describe("ResourceDetail", () => {
  it("renders tabs from descriptor detailPanels", () => {
    render(ResourceDetail, {
      props: {
        obj,
        descriptor: deployDescriptor,
        ctxName: "ctx",
        gvr: "apps.v1.deployments",
        namespace: "default",
        name: "my-deploy",
        onrefresh: vi.fn(),
      },
    });
    expect(screen.getByText("Overview")).toBeTruthy();
    expect(screen.getByText("Events")).toBeTruthy();
  });

  it("shows Overview panel by default", () => {
    render(ResourceDetail, {
      props: {
        obj,
        descriptor: deployDescriptor,
        ctxName: "ctx",
        gvr: "apps.v1.deployments",
        namespace: "default",
        name: "my-deploy",
        onrefresh: vi.fn(),
      },
    });
    // Overview fields should be visible
    expect(screen.getByText("Namespace")).toBeTruthy();
  });

  it("switches to Events panel on tab click", async () => {
    render(ResourceDetail, {
      props: {
        obj,
        descriptor: deployDescriptor,
        ctxName: "ctx",
        gvr: "apps.v1.deployments",
        namespace: "default",
        name: "my-deploy",
        onrefresh: vi.fn(),
      },
    });

    await fireEvent.click(screen.getByText("Events"));
    // EventsPanel renders when tab is selected (mock resolves with empty list → "No events found.")
    const {waitFor} = await import("@testing-library/svelte");
    await waitFor(() => expect(screen.getByText("No events found.")).toBeTruthy());
  });

  it("renders actions toolbar when actions are defined", () => {
    render(ResourceDetail, {
      props: {
        obj,
        descriptor: deployDescriptor,
        ctxName: "ctx",
        gvr: "apps.v1.deployments",
        namespace: "default",
        name: "my-deploy",
        onrefresh: vi.fn(),
      },
    });
    expect(screen.getByText("Scale")).toBeTruthy();
    expect(screen.getByText("Restart")).toBeTruthy();
    expect(screen.getByText("Delete")).toBeTruthy();
  });

  it("does not crash with unknown panel keys", () => {
    const descWithUnknown: DescriptorDef = {
      ...deployDescriptor,
      detailPanels: ["overview", "unknown-panel-xyz"],
    };
    // Should render without throwing — unknown panels are filtered out
    render(ResourceDetail, {
      props: {
        obj,
        descriptor: descWithUnknown,
        ctxName: "ctx",
        gvr: "apps.v1.deployments",
        namespace: "default",
        name: "my-deploy",
        onrefresh: vi.fn(),
      },
    });
    expect(screen.getByText("Overview")).toBeTruthy();
    expect(screen.queryByText("unknown-panel-xyz")).toBeNull();
  });
});
