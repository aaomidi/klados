// @ts-nocheck
// Web-transport facade for klados.v1.ClusterService.

import { cluster, fromJsonBytes } from "$lib/transport/clients";

export function ListContexts() {
    return cluster.listContexts({}).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function Connect(contextName) {
    return cluster.connect({ context: contextName }).then(() => undefined);
}

export function Disconnect(contextName) {
    return cluster.disconnect({ context: contextName }).then(() => undefined);
}

export function Activate(contextName) {
    return cluster.activate({ context: contextName }).then(() => undefined);
}

export function Deactivate(contextName) {
    return cluster.deactivate({ context: contextName }).then(() => undefined);
}

export function ListNamespaces(contextName) {
    return cluster.listNamespaces({ context: contextName }).then((r) => r.values);
}

export function SwitchNamespace(contextName, $namespace) {
    return cluster.switchNamespace({ context: contextName, namespace: $namespace }).then(() => undefined);
}

export function GetActiveNamespace(contextName) {
    return cluster.getActiveNamespace({ context: contextName }).then((r) => r.namespace);
}

export function GetStatus(contextName) {
    return cluster.getStatus({ context: contextName }).then((r) => r.status);
}

export function CreateNamespace(contextName, name) {
    return cluster.createNamespace({ context: contextName, name }).then(() => undefined);
}

export function DeleteNamespace(contextName, name) {
    return cluster.deleteNamespace({ context: contextName, name }).then(() => undefined);
}

export function AddKubeconfigPath(path) {
    return cluster.addKubeconfigPath({ path }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function ImportKubeconfigContent(content) {
    return cluster.importKubeconfigContent({ content }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function RemoveKubeconfigPath(path) {
    return cluster.removeKubeconfigPath({ path }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function GetClusterInfo(ctxName) {
    return cluster.getClusterInfo({ context: ctxName }).then((r) => fromJsonBytes(r.resultJson));
}
