// @ts-nocheck
// Web-transport facade for klados.v1.PluginService.

import { plugin, fromJsonBytes, toJsonBytes } from "$lib/transport/clients";

export function InvokeCommand(pluginName, commandID) {
    return plugin.invokeCommand({ pluginName, commandId: commandID }).then(() => undefined);
}

export function ReloadPlugin(name) {
    return plugin.reloadPluginManual({ name }).then(() => undefined);
}

export function ReloadPluginManual(name) {
    return plugin.reloadPluginManual({ name }).then(() => undefined);
}

export function EnablePlugin(name) {
    return plugin.enablePlugin({ name }).then(() => undefined);
}

export function DisablePlugin(name) {
    return plugin.disablePlugin({ name }).then(() => undefined);
}

export function UninstallPlugin(name) {
    return plugin.uninstallPlugin({ name }).then(() => undefined);
}

export function InstallPlugin(path) {
    return plugin.installPlugin({ path }).then(() => undefined);
}

/**
 * Web-only: install a plugin archive uploaded from the browser.
 * @param {Uint8Array} data
 * @param {string} name
 */
export function InstallPluginArchive(data, name) {
    return plugin.installPlugin({ archiveData: data, archiveName: name }).then(() => undefined);
}

export function PackPlugin(pluginDir) {
    return plugin.packPlugin({ pluginDir }).then((r) => r.archivePath);
}

export function SaveRegistryCredentials(host, username, password) {
    return plugin.saveRegistryCredentials({ host, username, password }).then(() => undefined);
}

export function AddInsecureRegistry(host) {
    return plugin.addInsecureRegistry({ host }).then(() => undefined);
}

export function EmitClusterEvent(eventName, payload) {
    const data = payload instanceof Uint8Array ? payload : toJsonBytes(payload);
    return plugin.emitClusterEvent({ eventName, payloadJson: data }).then(() => undefined);
}

export function GetPluginStorageKey(pluginName, key) {
    return plugin.getPluginStorageKey({ pluginName, key }).then((r) => r.value);
}

export function SetPluginStorageKey(pluginName, key, value) {
    return plugin.setPluginStorageKey({ pluginName, key, value }).then(() => undefined);
}

export function DeletePluginStorageKey(pluginName, key) {
    return plugin.deletePluginStorageKey({ pluginName, key }).then(() => undefined);
}

export function ListPluginStorageKeys(pluginName) {
    return plugin.listPluginStorageKeys({ name: pluginName }).then((r) => r.values);
}

export function ListPlugins() {
    return plugin.listPlugins({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginDescriptors() {
    return plugin.getPluginDescriptors({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginSidebarEntries() {
    return plugin.getPluginSidebarEntries({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginDetailTabs() {
    return plugin.getPluginDetailTabs({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginCommands() {
    return plugin.getPluginCommands({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginOverviewFields(gvr) {
    return plugin.getPluginOverviewFields({ gvr }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginListColumns(gvr) {
    return plugin.getPluginListColumns({ gvr }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginContextMenuItems(gvr) {
    return plugin.getPluginContextMenuItems({ gvr }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginHeaderWidgets() {
    return plugin.getPluginHeaderWidgets({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginStatusBarWidgets() {
    return plugin.getPluginStatusBarWidgets({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginMetricQueries(gvr) {
    return plugin.getPluginMetricQueries({ gvr }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetPluginSettings(name) {
    return plugin.getPluginSettings({ name }).then((r) => r.value);
}

export function SetPluginSettings(name, settingsJSON) {
    return plugin.setPluginSettings({ name, settingsJson: settingsJSON }).then(() => undefined);
}

export function GetPluginSettingsSchema(name) {
    return plugin.getPluginSettingsSchema({ name }).then((r) => r.value);
}
