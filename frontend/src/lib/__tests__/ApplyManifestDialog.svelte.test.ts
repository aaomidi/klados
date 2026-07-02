import {describe, it, expect, vi} from "vitest";
import {render, screen, waitFor, fireEvent} from "@testing-library/svelte";
import {tick} from "svelte";

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  ApplyManifest: vi.fn().mockResolvedValue([
    {gvr: "core.v1.configmaps", namespace: "default", name: "my-cm", action: "created", error: ""},
    {gvr: "apps.v1.deployments", namespace: "default", name: "my-app", action: "configured", error: ""},
  ]),
}));

vi.mock("$lib/stores/capabilities.svelte.js", () => ({
  capabilitiesStore: {nativeDialogs: true, osWindows: true, portForwarding: true, mode: "desktop", loaded: true},
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/appservice.js", () => ({
  BrowseManifestFile: vi.fn().mockResolvedValue("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: file-cm\n"),
  LogFrontend: vi.fn(),
  GetCapabilities: vi.fn().mockResolvedValue({nativeDialogs: true, osWindows: true, portForwarding: true, mode: "desktop"}),
}));

vi.mock("$lib/stores/notification.svelte", () => ({
  notificationStore: {push: vi.fn()},
}));

import ApplyManifestDialog from "$lib/components/ApplyManifestDialog.svelte";

const APPLY_REGEX = /Apply/i;

describe("ApplyManifestDialog", () => {
  it("renders Open File and Paste from Clipboard buttons when open", async () => {
    render(ApplyManifestDialog, {props: {open: true, ctxName: "test-ctx"}});
    await tick();
    expect(screen.getByText("Open File…")).toBeTruthy();
    expect(screen.getByText("Paste from Clipboard")).toBeTruthy();
  });

  it("Apply button is disabled when editor is empty", async () => {
    render(ApplyManifestDialog, {props: {open: true, ctxName: "test-ctx"}});
    await tick();
    await waitFor(() => {
      const btn = screen.getByRole("button", {name: APPLY_REGEX});
      expect(btn).toBeTruthy();
      expect((btn as HTMLButtonElement).disabled).toBe(true);
    });
  });

  it("shows results section after Apply is clicked", async () => {
    render(ApplyManifestDialog, {props: {open: true, ctxName: "test-ctx"}});
    await tick();

    // Simulate paste from clipboard
    Object.defineProperty(navigator, "clipboard", {
      value: {readText: vi.fn().mockResolvedValue("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n")},
      writable: true,
      configurable: true,
    });
    const pasteBtn = screen.getByText("Paste from Clipboard");
    await fireEvent.click(pasteBtn);
    await tick();

    // Find and click Apply button when enabled
    await waitFor(async () => {
      const btns = screen.getAllByRole("button");
      const applyBtn = btns.find((b) => b.textContent?.includes("Apply") && !(b as HTMLButtonElement).disabled);
      if (!applyBtn) {
        throw new Error("Apply button not enabled yet");
      }
      await fireEvent.click(applyBtn);
    });

    await waitFor(() => {
      expect(screen.getByText("created")).toBeTruthy();
    });
  });

  it("shows Cancel button", async () => {
    render(ApplyManifestDialog, {props: {open: true, ctxName: "test-ctx"}});
    await tick();
    expect(screen.getByText("Cancel")).toBeTruthy();
  });
});
