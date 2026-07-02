// @ts-nocheck
// Web-transport facade for klados.v1.ConfigService.

import { configSvc, fromJsonBytes, toJsonBytes } from "$lib/transport/clients";

export function GetTheme() {
    return configSvc.getTheme({}).then((r) => r.value);
}

export function SetTheme(theme) {
    return configSvc.setTheme({ theme }).then(() => undefined);
}

export function GetTerminalWebGL() {
    return configSvc.getTerminalWebGL({}).then((r) => r.value);
}

export function SetTerminalWebGL(enabled) {
    return configSvc.setTerminalWebGL({ enabled }).then(() => undefined);
}

export function GetInsecureSkipTLSVerify() {
    return configSvc.getInsecureSkipTLSVerify({}).then((r) => r.value);
}

export function SetInsecureSkipTLSVerify(skip) {
    return configSvc.setInsecureSkipTLSVerify({ enabled: skip }).then(() => undefined);
}

export function GetCompactRows() {
    return configSvc.getCompactRows({}).then((r) => r.value);
}

export function SetCompactRows(compact) {
    return configSvc.setCompactRows({ enabled: compact }).then(() => undefined);
}

export function SetContextualAutocomplete(enabled) {
    return configSvc.setContextualAutocomplete({ enabled }).then(() => undefined);
}

export function GetConfig() {
    return configSvc.getConfig({}).then((r) => fromJsonBytes(r.resultJson));
}

export function GetResolvedPrefs(ctxName) {
    return configSvc.getResolvedPrefs({ context: ctxName }).then((r) => fromJsonBytes(r.resultJson));
}

export function GetColumnPrefs(gvr) {
    return configSvc.getColumnPrefs({ gvr }).then((r) => fromJsonBytes(r.resultJson));
}

export function SetColumnPrefs(gvr, prefs) {
    return configSvc.setColumnPrefs({ gvr, prefsJson: toJsonBytes(prefs) }).then(() => undefined);
}

export function DeleteColumnPrefs(gvr) {
    return configSvc.deleteColumnPrefs({ gvr }).then(() => undefined);
}

export function SetAccentColor(color) {
    return configSvc.setAccentColor({ value: color }).then(() => undefined);
}

export function SetFontSize(size) {
    return configSvc.setFontSize({ size }).then(() => undefined);
}

export function SetStartupBehavior(behavior, cluster) {
    return configSvc.setStartupBehavior({ behavior, cluster }).then(() => undefined);
}

export function SetKeybinding(actionID, keys) {
    return configSvc.setKeybinding({ actionId: actionID, keys }).then(() => undefined);
}

export function ResetKeybindings() {
    return configSvc.resetKeybindings({}).then(() => undefined);
}

export function GetClusterPrefs(ctxName) {
    return configSvc.getClusterPrefs({ context: ctxName }).then((r) => fromJsonBytes(r.resultJson));
}

export function SetClusterPrefs(ctxName, prefs) {
    return configSvc.setClusterPrefs({ context: ctxName, prefsJson: toJsonBytes(prefs) }).then(() => undefined);
}

export function DeleteClusterPrefs(ctxName) {
    return configSvc.deleteClusterPrefs({ context: ctxName }).then(() => undefined);
}

export function GetSavedFilters(gvr) {
    return configSvc.getSavedFilters({ gvr }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function SetSavedFilters(gvr, filters) {
    return configSvc.setSavedFilters({ gvr, filtersJson: toJsonBytes(filters ?? []) }).then(() => undefined);
}

export function SetClusterSavedFilters(ctxName, gvr, filters) {
    return configSvc
        .setClusterSavedFilters({ context: ctxName, gvr, filtersJson: toJsonBytes(filters ?? []) })
        .then(() => undefined);
}

export function SetVolumeBrowser(vb) {
    return configSvc.setVolumeBrowser({ configJson: toJsonBytes(vb) }).then(() => undefined);
}
