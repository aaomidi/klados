import {describe, it, expect, vi, beforeEach} from "vitest";
import {render, screen, waitFor, fireEvent} from "@testing-library/svelte";

const {mockInstallPlugin, mockListPlugins, mockSaveRegistryCredentials, mockAddInsecureRegistry, mockBrowsePluginFile} = vi.hoisted(() => ({
  mockInstallPlugin: vi.fn(),
  mockListPlugins: vi.fn().mockResolvedValue([]),
  mockSaveRegistryCredentials: vi.fn().mockResolvedValue(undefined),
  mockAddInsecureRegistry: vi.fn().mockResolvedValue(undefined),
  mockBrowsePluginFile: vi.fn().mockResolvedValue(null),
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/pluginservice.js", () => ({
  InstallPlugin: mockInstallPlugin,
  ListPlugins: mockListPlugins,
  SaveRegistryCredentials: mockSaveRegistryCredentials,
  AddInsecureRegistry: mockAddInsecureRegistry,
  EnablePlugin: vi.fn().mockResolvedValue(undefined),
  DisablePlugin: vi.fn().mockResolvedValue(undefined),
  ReloadPluginManual: vi.fn().mockResolvedValue(undefined),
  UninstallPlugin: vi.fn().mockResolvedValue(undefined),
  GetPluginDescriptors: vi.fn().mockResolvedValue([]),
  GetPluginSidebarEntries: vi.fn().mockResolvedValue([]),
  GetPluginDetailTabs: vi.fn().mockResolvedValue([]),
  GetPluginCommands: vi.fn().mockResolvedValue([]),
}));

vi.mock("$lib/stores/capabilities.svelte.js", () => ({
  capabilitiesStore: {nativeDialogs: true, osWindows: true, portForwarding: true, mode: "desktop", loaded: true},
}));

vi.mock("$api/github.com/Vilsol/klados/internal/services/appservice.js", () => ({
  BrowsePluginFile: mockBrowsePluginFile,
  GetCapabilities: vi.fn().mockResolvedValue({nativeDialogs: true, osWindows: true, portForwarding: true, mode: "desktop"}),
}));

import PluginManagement from "../../routes/PluginManagement.svelte";

describe("PluginManagement — registry install", () => {
  beforeEach(() => {
    mockInstallPlugin.mockReset();
    mockListPlugins.mockResolvedValue([]);
    mockSaveRegistryCredentials.mockResolvedValue(undefined);
    mockAddInsecureRegistry.mockResolvedValue(undefined);
  });

  it("renders registry input and install button", () => {
    render(PluginManagement);

    expect(screen.getByPlaceholderText("ghcr.io/owner/plugin:v1")).toBeTruthy();
    expect(screen.getByText("Install")).toBeTruthy();
  });

  it("happy path: InstallPlugin resolves and plugins reload", async () => {
    mockInstallPlugin.mockResolvedValue(undefined);
    render(PluginManagement);

    const input = screen.getByPlaceholderText("ghcr.io/owner/plugin:v1");
    await fireEvent.input(input, {target: {value: "ghcr.io/owner/plugin:v1"}});

    const btn = screen.getByText("Install");
    await fireEvent.click(btn);

    await waitFor(() => expect(mockInstallPlugin).toHaveBeenCalledWith("oci://ghcr.io/owner/plugin:v1"));
    await waitFor(() => expect(mockListPlugins).toHaveBeenCalled());
  });

  it("normalises bare ref by prepending oci://", async () => {
    mockInstallPlugin.mockResolvedValue(undefined);
    render(PluginManagement);

    const input = screen.getByPlaceholderText("ghcr.io/owner/plugin:v1");
    await fireEvent.input(input, {target: {value: "ghcr.io/foo/bar:v1"}});
    await fireEvent.click(screen.getByText("Install"));

    await waitFor(() => expect(mockInstallPlugin).toHaveBeenCalledWith("oci://ghcr.io/foo/bar:v1"));
  });

  it("shows auth form with pre-filled host when authentication required", async () => {
    mockInstallPlugin.mockRejectedValue(new Error("authentication required"));
    render(PluginManagement);

    const input = screen.getByPlaceholderText("ghcr.io/owner/plugin:v1");
    await fireEvent.input(input, {target: {value: "ghcr.io/owner/plugin:v1"}});
    await fireEvent.click(screen.getByText("Install"));

    await waitFor(() => expect(screen.getByText("Save & Retry")).toBeTruthy());
    expect(screen.getByText("ghcr.io")).toBeTruthy();
  });

  it("submitCredentials calls SaveRegistryCredentials then retries InstallPlugin", async () => {
    mockInstallPlugin.mockRejectedValueOnce(new Error("authentication required")).mockResolvedValueOnce(undefined);
    render(PluginManagement);

    const input = screen.getByPlaceholderText("ghcr.io/owner/plugin:v1");
    await fireEvent.input(input, {target: {value: "ghcr.io/owner/plugin:v1"}});
    await fireEvent.click(screen.getByText("Install"));
    await waitFor(() => expect(screen.getByText("Save & Retry")).toBeTruthy());

    const userInput = screen.getByPlaceholderText("Username");
    const passInput = screen.getByPlaceholderText("Password or token");
    await fireEvent.input(userInput, {target: {value: "myuser"}});
    await fireEvent.input(passInput, {target: {value: "mytoken"}});

    await fireEvent.click(screen.getByText("Save & Retry"));

    await waitFor(() => expect(mockSaveRegistryCredentials).toHaveBeenCalledWith("ghcr.io", "myuser", "mytoken"));
    expect(mockInstallPlugin).toHaveBeenCalledTimes(2);
    expect(mockInstallPlugin).toHaveBeenLastCalledWith("oci://ghcr.io/owner/plugin:v1");
  });

  it("calls AddInsecureRegistry when insecure checkbox is checked", async () => {
    mockInstallPlugin.mockRejectedValueOnce(new Error("authentication required")).mockResolvedValueOnce(undefined);
    render(PluginManagement);

    const input = screen.getByPlaceholderText("ghcr.io/owner/plugin:v1");
    await fireEvent.input(input, {target: {value: "ghcr.io/owner/plugin:v1"}});
    await fireEvent.click(screen.getByText("Install"));
    await waitFor(() => expect(screen.getByText("Save & Retry")).toBeTruthy());

    const checkbox = screen.getByRole("checkbox");
    await fireEvent.click(checkbox);
    await fireEvent.click(screen.getByText("Save & Retry"));

    await waitFor(() => expect(mockAddInsecureRegistry).toHaveBeenCalledWith("ghcr.io"));
  });

  it("shows error message when credentials are rejected on retry", async () => {
    mockInstallPlugin.mockRejectedValue(new Error("authentication required"));
    render(PluginManagement);

    const input = screen.getByPlaceholderText("ghcr.io/owner/plugin:v1");
    await fireEvent.input(input, {target: {value: "ghcr.io/owner/plugin:v1"}});
    await fireEvent.click(screen.getByText("Install"));
    await waitFor(() => expect(screen.getByText("Save & Retry")).toBeTruthy());

    await fireEvent.click(screen.getByText("Save & Retry"));

    await waitFor(() => expect(screen.getByText("Credentials rejected — verify and try again")).toBeTruthy());
    expect(screen.getByText("Save & Retry")).toBeTruthy();
  });
});
