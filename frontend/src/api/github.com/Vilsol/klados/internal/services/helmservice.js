// @ts-nocheck
// Web-transport facade for klados.v1.HelmService.

import { helm, fromJsonBytes } from "$lib/transport/clients";

export function Rollback(contextName, $namespace, releaseName, revision, opts) {
    const o = opts ?? {};
    return helm
        .rollback({
            context: contextName,
            namespace: $namespace,
            releaseName,
            revision,
            wait: !!o.wait,
            timeout: o.timeout ?? 0,
            disableHooks: !!o.disableHooks,
        })
        .then(() => undefined);
}

export function Uninstall(contextName, $namespace, releaseName, opts) {
    const o = opts ?? {};
    return helm
        .uninstall({
            context: contextName,
            namespace: $namespace,
            releaseName,
            wait: !!o.wait,
            timeout: o.timeout ?? 0,
            disableHooks: !!o.disableHooks,
            keepHistory: !!o.keepHistory,
        })
        .then(() => undefined);
}

export function Test(contextName, $namespace, releaseName, opts) {
    const o = opts ?? {};
    return helm
        .test({
            context: contextName,
            namespace: $namespace,
            releaseName,
            timeout: o.timeout ?? 0,
            filters: o.filters ?? [],
        })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function ForceDeleteReleaseSecret(contextName, $namespace, releaseName, revision) {
    return helm
        .forceDeleteReleaseSecret({ context: contextName, namespace: $namespace, releaseName, revision })
        .then(() => undefined);
}

export function GetValues(contextName, $namespace, releaseName, computed, revision) {
    return helm
        .getValues({ context: contextName, namespace: $namespace, releaseName, computed, revision })
        .then((r) => r.text);
}

export function GetManifest(contextName, $namespace, releaseName, revision) {
    return helm
        .getManifest({ context: contextName, namespace: $namespace, releaseName, revision })
        .then((r) => r.text);
}

export function GetHistory(contextName, $namespace, releaseName) {
    return helm
        .getHistory({ context: contextName, namespace: $namespace, releaseName })
        .then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetNotes(contextName, $namespace, releaseName, revision) {
    return helm
        .getNotes({ context: contextName, namespace: $namespace, releaseName, revision })
        .then((r) => r.text);
}

export function GetHooks(contextName, $namespace, releaseName, revision) {
    return helm
        .getHooks({ context: contextName, namespace: $namespace, releaseName, revision })
        .then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetOwnedResources(contextName, $namespace, releaseName, scanAll) {
    return helm
        .getOwnedResources({ context: contextName, namespace: $namespace, releaseName, scanAll })
        .then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function DiffRevisions(contextName, $namespace, releaseName, from, to) {
    return helm
        .diffRevisions({ context: contextName, namespace: $namespace, releaseName, from, to })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function GetReleasePermissions(contextName, $namespace, releaseName) {
    return helm
        .getReleasePermissions({ context: contextName, namespace: $namespace, releaseName })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function CleanupTestPods(contextName, $namespace, releaseName) {
    return helm
        .cleanupTestPods({ context: contextName, namespace: $namespace, releaseName })
        .then(() => undefined);
}
