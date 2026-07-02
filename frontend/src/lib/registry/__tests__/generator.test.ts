import { describe, it, expect } from "vitest";
import {
  jsonPathToCEL,
  crdTypeToRenderType,
  generateDescriptor,
} from "../generator";
import type { APIResource } from "$api/github.com/Vilsol/klados/internal/cluster/index.js";

describe("jsonPathToCEL", () => {
  it("strips leading dot", () => {
    expect(jsonPathToCEL(".spec.replicas")).toBe("spec.replicas");
  });

  it("preserves deep dotted paths", () => {
    expect(jsonPathToCEL(".status.loadBalancer.ingress")).toBe("status.loadBalancer.ingress");
  });

  it("passes through root $", () => {
    expect(jsonPathToCEL("$.metadata.name")).toBe("metadata.name");
  });

  it("returns empty for empty input", () => {
    expect(jsonPathToCEL("")).toBe("");
  });

  it("returns empty for unsupported filter expressions", () => {
    expect(jsonPathToCEL('.status.conditions[?(@.type=="Ready")].status')).toBe("");
  });
});

describe("crdTypeToRenderType", () => {
  it("maps date → age", () => {
    expect(crdTypeToRenderType("date")).toBe("age");
  });
  it("maps boolean → badge", () => {
    expect(crdTypeToRenderType("boolean")).toBe("badge");
  });
  it("maps string/integer/number → text", () => {
    expect(crdTypeToRenderType("string")).toBe("text");
    expect(crdTypeToRenderType("integer")).toBe("text");
    expect(crdTypeToRenderType("number")).toBe("text");
  });
  it("defaults unknown → text", () => {
    expect(crdTypeToRenderType("")).toBe("text");
    expect(crdTypeToRenderType("gibberish")).toBe("text");
  });
});

const baseResource = (over: Partial<APIResource> = {}): APIResource => ({
  gvr: "example.com.v1.widgets",
  kind: "Widget",
  namespaced: true,
  subresources: { scale: false, status: false },
  printerColumns: [],
  scaleSpec: undefined,
  ...over,
}) as unknown as APIResource;

describe("generateDescriptor", () => {
  it("prepends Name/Namespace, appends printer columns, and keeps Age last", () => {
    const d = generateDescriptor(baseResource({
      printerColumns: [
        { name: "Replicas", type: "integer", jsonPath: ".spec.replicas", priority: 0 },
        { name: "Ready", type: "string", jsonPath: ".status.ready", priority: 1 },
      ] as APIResource["printerColumns"],
    }));

    expect(d.columns.map((c) => c.name)).toEqual(["Name", "Namespace", "Replicas", "Ready", "Age"]);
    expect(d.columns[2].expr).toBe("spec.replicas");
    expect(d.columns[2].renderType).toBe("text");
    expect(d.columns[3].hidden).toBe(true);
  });

  it("omits Namespace column for cluster-scoped resources", () => {
    const d = generateDescriptor(baseResource({ namespaced: false }));
    expect(d.columns.map((c) => c.name)).toEqual(["Name", "Age"]);
    expect(d.clusterScoped).toBe(true);
  });

  it("skips printer columns with unsupported JSONPath", () => {
    const d = generateDescriptor(baseResource({
      printerColumns: [
        { name: "Filtered", type: "string", jsonPath: '.status.conditions[?(@.type=="Ready")].status', priority: 0 },
      ] as APIResource["printerColumns"],
    }));
    expect(d.columns.map((c) => c.name)).toEqual(["Name", "Namespace", "Age"]);
  });

  it("adds Scale action when scale subresource present", () => {
    const d = generateDescriptor(baseResource({
      subresources: { scale: true, status: false },
      scaleSpec: { specReplicasPath: ".spec.replicas", statusReplicasPath: ".status.replicas" },
    }));
    expect(d.actions!.some((a) => a.name === "scale")).toBe(true);
    expect(d.columns.some((c) => c.name === "Replicas")).toBe(true);
  });

  it("does not duplicate Replicas column when printer columns already include one", () => {
    const d = generateDescriptor(baseResource({
      subresources: { scale: true, status: false },
      scaleSpec: { specReplicasPath: ".spec.replicas", statusReplicasPath: ".status.replicas" },
      printerColumns: [
        { name: "Replicas", type: "integer", jsonPath: ".spec.replicas", priority: 0 },
      ] as APIResource["printerColumns"],
    }));
    const replicaCols = d.columns.filter((c) => c.name === "Replicas");
    expect(replicaCols.length).toBe(1);
  });

  it("always includes universal detail panels", () => {
    const d = generateDescriptor(baseResource());
    expect(d.detailPanels).toEqual(expect.arrayContaining([
      "overview", "yaml", "events", "conditions", "metadata", "related", "drift",
    ]));
  });

  it("adds 'status' panel when status subresource present", () => {
    const d = generateDescriptor(baseResource({
      subresources: { scale: false, status: true },
    }));
    expect(d.detailPanels).toContain("status");
  });

  it("always includes delete action and edit-yaml action", () => {
    const d = generateDescriptor(baseResource());
    const names = d.actions!.map((a) => a.name);
    expect(names).toContain("delete");
    expect(names).toContain("edit-yaml");
  });
});
