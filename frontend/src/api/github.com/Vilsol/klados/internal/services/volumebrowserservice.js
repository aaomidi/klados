// @ts-nocheck
// Web-transport facade for klados.v1.VolumeBrowserService.

import { volumebrowser, fromJsonBytes, toJsonBytes } from "$lib/transport/clients";

export function Spawn(req) {
    return volumebrowser.spawn({ requestJson: toJsonBytes(req) }).then((r) => fromJsonBytes(r.resultJson));
}

export function Stop(id) {
    return volumebrowser.stop({ id }).then(() => undefined);
}

export function Replace(id, req) {
    return volumebrowser.replace({ id, requestJson: toJsonBytes(req) }).then((r) => fromJsonBytes(r.resultJson));
}

export function AttachTab(id, tabID) {
    return volumebrowser.attachTab({ id, tabId: tabID }).then(() => undefined);
}

export function ListManaged(contextName) {
    return volumebrowser.listManaged({ context: contextName }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function ScanOrphans(contextName) {
    return volumebrowser.scanOrphans({ context: contextName }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function CleanupOrphans(contextName) {
    return volumebrowser.cleanupOrphans({ context: contextName }).then(() => undefined);
}

export function TriggerOrphanScan(contextName) {
    return volumebrowser.triggerOrphanScan({ context: contextName }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}
