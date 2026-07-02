// @ts-nocheck
// Web-transport facade: same exports as the old Wails bindings, backed by
// the klados.v1.ResourceService Connect client.

import { resource, fromJsonBytes, toJsonBytes } from "$lib/transport/clients";

export function ApplyManifest(contextName, yamlContent) {
    return resource.applyManifest({ context: contextName, yamlContent }).then((r) => fromJsonBytes(r.resultJson));
}

export function CreateResource(contextName, gvr, $namespace, obj) {
    return resource
        .createResource({ context: contextName, gvr, namespace: $namespace, objectJson: toJsonBytes(obj) })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function UpdateResource(contextName, gvr, $namespace, obj) {
    return resource
        .updateResource({ context: contextName, gvr, namespace: $namespace, objectJson: toJsonBytes(obj) })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function DeleteResource(contextName, gvr, $namespace, name) {
    return resource.deleteResource({ context: contextName, gvr, namespace: $namespace, name }).then(() => undefined);
}

export function ForceDeleteResource(contextName, gvr, $namespace, name) {
    return resource.forceDeleteResource({ context: contextName, gvr, namespace: $namespace, name }).then(() => undefined);
}

export function DeleteJobCascade(contextName, $namespace, name) {
    return resource.deleteJobCascade({ context: contextName, namespace: $namespace, name }).then(() => undefined);
}

export function DeleteJobOrphan(contextName, $namespace, name) {
    return resource.deleteJobOrphan({ context: contextName, namespace: $namespace, name }).then(() => undefined);
}

export function ExpandPVC(contextName, $namespace, name, newSize) {
    return resource.expandPVC({ context: contextName, namespace: $namespace, name, newSize }).then(() => undefined);
}

export function GetAllTemplateGVRs(contextName) {
    return resource.getAllTemplateGVRs({ context: contextName }).then((r) => r.values);
}

export function GetDescriptors() {
    return resource.getDescriptors({}).then((r) => fromJsonBytes(r.resultJson));
}

export function GetEvents(contextName, $namespace, uid) {
    return resource.getEvents({ context: contextName, namespace: $namespace, uid }).then((r) => fromJsonBytes(r.resultJson));
}

export function GetResource(contextName, gvr, $namespace, name) {
    return resource.getResource({ context: contextName, gvr, namespace: $namespace, name }).then((r) => fromJsonBytes(r.resultJson));
}

export function GetRolloutHistory(contextName, gvr, $namespace, name) {
    return resource
        .getRolloutHistory({ context: contextName, gvr, namespace: $namespace, name })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function GetTemplates(contextName, gvr) {
    return resource.getTemplates({ context: contextName, gvr }).then((r) => fromJsonBytes(r.resultJson));
}

export function ListAPIResources(contextName) {
    return resource.listAPIResources({ context: contextName }).then((r) => fromJsonBytes(r.resultJson));
}

export function ListResources(contextName, gvr, $namespace) {
    return resource
        .listResourcesWithVersion({ context: contextName, gvr, namespace: $namespace })
        .then((r) => fromJsonBytes(r.itemsJson) ?? []);
}

export function ListResourcesWithVersion(contextName, gvr, $namespace) {
    return resource.listResourcesWithVersion({ context: contextName, gvr, namespace: $namespace }).then((r) => ({
        items: fromJsonBytes(r.itemsJson) ?? [],
        resourceVersion: r.resourceVersion,
    }));
}

export function PauseRollout(contextName, $namespace, name) {
    return resource.pauseRollout({ context: contextName, namespace: $namespace, name }).then(() => undefined);
}

export function ResumeRollout(contextName, $namespace, name) {
    return resource.resumeRollout({ context: contextName, namespace: $namespace, name }).then(() => undefined);
}

export function RestartResource(contextName, gvr, $namespace, name) {
    return resource.restartResource({ context: contextName, gvr, namespace: $namespace, name }).then(() => undefined);
}

export function RollbackToRevision(contextName, gvr, $namespace, name, revision) {
    return resource
        .rollbackToRevision({ context: contextName, gvr, namespace: $namespace, name, revision: BigInt(revision) })
        .then(() => undefined);
}

export function ScaleResource(contextName, gvr, $namespace, name, replicas) {
    return resource.scaleResource({ context: contextName, gvr, namespace: $namespace, name, replicas }).then(() => undefined);
}

export function StartWatch(contextName, gvr, $namespace, resourceVersion) {
    return resource.startWatch({ context: contextName, gvr, namespace: $namespace, resourceVersion }).then(() => undefined);
}

export function StopWatch(contextName, gvr, $namespace) {
    return resource.stopWatch({ context: contextName, gvr, namespace: $namespace }).then(() => undefined);
}

export function SuspendCronJob(contextName, $namespace, name) {
    return resource.suspendCronJob({ context: contextName, namespace: $namespace, name }).then(() => undefined);
}

export function ResumeCronJob(contextName, $namespace, name) {
    return resource.resumeCronJob({ context: contextName, namespace: $namespace, name }).then(() => undefined);
}

export function TriggerCronJob(contextName, $namespace, name) {
    return resource.triggerCronJob({ context: contextName, namespace: $namespace, name }).then(() => undefined);
}

// Internal-accessor bindings from the desktop build; not part of the web API.
function unavailable(name) {
    return () => Promise.reject(new Error(`${name} is not available in the web build`));
}

export const Engine = unavailable("ResourceService.Engine");
export const EnricherRegistry = unavailable("ResourceService.EnricherRegistry");
export const Registry = unavailable("ResourceService.Registry");
export const WatchMgr = unavailable("ResourceService.WatchMgr");
