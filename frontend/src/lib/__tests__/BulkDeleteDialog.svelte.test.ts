import {describe, it, expect, beforeEach, vi} from "vitest";
import {render, screen, fireEvent, waitFor} from "@testing-library/svelte";

const mockDeleteResource = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const mockForceDeleteResource = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const mockDeselectKeys = vi.hoisted(() => vi.fn());
const mockNotificationPush = vi.hoisted(() => vi.fn());
const mockItems = vi.hoisted(() => vi.fn());

vi.mock("$api/github.com/Vilsol/klados/internal/services/resourceservice.js", () => ({
  DeleteResource: mockDeleteResource,
  ForceDeleteResource: mockForceDeleteResource,
}));

vi.mock("$lib/stores/selection.svelte", () => ({
  selectionStore: {
    items: mockItems,
    selectedGVR: "core.v1.pods",
    deselectKeys: mockDeselectKeys,
  },
}));

vi.mock("$lib/stores/notification.svelte", () => ({
  notificationStore: {push: mockNotificationPush},
}));

vi.mock("$lib/stores/shortcutActions.svelte", () => ({
  shortcutActions: {confirmDialog: 0},
}));

import BulkDeleteDialog from "$lib/components/BulkDeleteDialog.svelte";

const fakeItems = [
  {metadata: {namespace: "default", name: "pod-a"}},
  {metadata: {namespace: "default", name: "pod-b"}},
];

function renderDialog() {
  return render(BulkDeleteDialog, {props: {open: true, contextName: "test-ctx"}});
}

describe("BulkDeleteDialog", () => {
  beforeEach(() => {
    mockItems.mockReturnValue(fakeItems);
    mockDeleteResource.mockReset();
    mockDeleteResource.mockResolvedValue(undefined);
    mockForceDeleteResource.mockReset();
    mockForceDeleteResource.mockResolvedValue(undefined);
    mockDeselectKeys.mockReset();
    mockNotificationPush.mockReset();
  });

  it("calls DeleteResource for each item when force is not checked", async () => {
    renderDialog();

    const confirmBtn = screen.getByRole("button", {name: /Delete 2 items/i});
    await fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockDeleteResource).toHaveBeenCalledTimes(2);
      expect(mockDeleteResource).toHaveBeenCalledWith("test-ctx", "core.v1.pods", "default", "pod-a");
      expect(mockDeleteResource).toHaveBeenCalledWith("test-ctx", "core.v1.pods", "default", "pod-b");
      expect(mockForceDeleteResource).not.toHaveBeenCalled();
    });
  });

  it("calls ForceDeleteResource for each item when force checkbox is checked", async () => {
    renderDialog();

    const checkbox = screen.getByRole("checkbox");
    await fireEvent.click(checkbox);

    const confirmBtn = screen.getByRole("button", {name: /Force Delete 2 items/i});
    await fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockForceDeleteResource).toHaveBeenCalledTimes(2);
      expect(mockForceDeleteResource).toHaveBeenCalledWith("test-ctx", "core.v1.pods", "default", "pod-a");
      expect(mockForceDeleteResource).toHaveBeenCalledWith("test-ctx", "core.v1.pods", "default", "pod-b");
      expect(mockDeleteResource).not.toHaveBeenCalled();
    });
  });

  it("button label changes from Delete to Force Delete when checkbox is toggled", async () => {
    renderDialog();

    expect(screen.getByRole("button", {name: /^Delete 2 items$/i})).toBeTruthy();

    const checkbox = screen.getByRole("checkbox");
    await fireEvent.click(checkbox);

    expect(screen.getByRole("button", {name: /^Force Delete 2 items$/i})).toBeTruthy();
  });
});
