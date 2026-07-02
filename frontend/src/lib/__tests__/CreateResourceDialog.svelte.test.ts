import {describe, it, expect, beforeEach, afterEach, vi} from "vitest";
import {render, screen, waitFor, cleanup} from "@testing-library/svelte";
import {tick} from "svelte";

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  CreateResource: vi.fn().mockResolvedValue({}),
  GetTemplates: vi
    .fn()
    .mockResolvedValue([
      {gvr: "core.v1.pods", name: "Basic Pod", description: "A basic pod", content: "apiVersion: v1\nkind: Pod\n", source: "builtin"},
    ]),
  GetAllTemplateGVRs: vi.fn().mockResolvedValue(["core.v1.pods", "apps.v1.deployments"]),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/schemaservice.js", () => ({
  GetSchema: vi.fn().mockResolvedValue({}),
}));

vi.mock("$lib/stores/notification.svelte", () => ({
  notificationStore: {push: vi.fn()},
}));

import CreateResourceDialog from "$lib/components/CreateResourceDialog.svelte";

describe("CreateResourceDialog", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runAllTimers();
    cleanup();
    vi.useRealTimers();
  });

  it("renders Resource Type and Template labels when open", async () => {
    render(CreateResourceDialog, {props: {open: true, ctxName: "test-ctx"}});
    await tick();
    await waitFor(() => {
      expect(screen.getByText("Resource Type")).toBeTruthy();
    });
  });

  it("pre-fills GVR combobox when gvr prop provided", async () => {
    render(CreateResourceDialog, {
      props: {open: true, ctxName: "test-ctx", gvr: "core.v1.pods"},
    });
    await tick();
    await waitFor(() => {
      // Combobox displays the selected value as visible text in a span overlay
      const matches = document.querySelectorAll("span");
      const found = Array.from(matches).some((el) => el.textContent === "core.v1.pods");
      expect(found).toBe(true);
    });
  });

  it("shows Template label when a GVR is selected", async () => {
    render(CreateResourceDialog, {
      props: {open: true, ctxName: "test-ctx", gvr: "core.v1.pods"},
    });
    await tick();
    await waitFor(() => {
      expect(screen.getByText("Template")).toBeTruthy();
    });
  });
});
