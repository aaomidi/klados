// @ts-nocheck
// Web-transport facade for klados.v1.AppService.

import { app, fromJsonBytes, toJsonBytes } from "$lib/transport/clients";

export function GetSession() {
    return app.getSession({}).then((r) => fromJsonBytes(r.resultJson));
}

export function SaveUIState(openTabs, activeTab, sidebarCollapsed, terminalFontSize, sidebarWidth) {
    return app
        .saveUIState({
            openTabsJson: toJsonBytes(openTabs ?? []),
            activeTab,
            sidebarCollapsed,
            terminalFontSize,
            sidebarWidth,
        })
        .then(() => undefined);
}

export function LogFrontend(level, message, attrsJSON) {
    return app.logFrontend({ level, message, attrsJson: attrsJSON }).then(() => undefined);
}

export function SetReadOnly(enabled) {
    return app.setReadOnly({ enabled }).then(() => undefined);
}

export function SetLastActiveContext(name) {
    return app.setLastActiveContext({ context: name }).then(() => undefined);
}

export function GetClusterHealth(connCtx) {
    return app.getClusterHealth({ context: connCtx }).then((r) => fromJsonBytes(r.resultJson));
}

export function GetCapabilities() {
    return app.getCapabilities({});
}

// The streaming server is same-origin in the web build; port/token are
// vestigial. Kept for call-site compatibility during the transition.
export function GetStreamingConfig() {
    return Promise.resolve({
        port: Number(window.location.port) || (window.location.protocol === "https:" ? 443 : 80),
        token: "",
    });
}

// Native dialogs — desktop only (GetCapabilities().nativeDialogs); the
// server rejects these with CodeUnimplemented.
export function BrowseKubeconfigFile() {
    return app.browseKubeconfigFile({}).then((r) => r.value);
}

export function BrowseManifestFile() {
    return app.browseManifestFile({}).then((r) => r.value);
}

export function BrowsePluginFile() {
    return app.browsePluginFile({}).then((r) => r.value);
}

function unavailable(name) {
    return () => Promise.reject(new Error(`${name} is not available in the web build`));
}

export const ClusterManager = unavailable("AppService.ClusterManager");
export const Config = unavailable("AppService.Config");
export const Ctx = unavailable("AppService.Ctx");
export const ExecManager = unavailable("AppService.ExecManager");
export const LogStreamer = unavailable("AppService.LogStreamer");
export const PortForwardManager = unavailable("AppService.PortForwardManager");
export const VolumeBrowserManager = unavailable("AppService.VolumeBrowserManager");
export const RegisterPluginsDir = unavailable("AppService.RegisterPluginsDir");
