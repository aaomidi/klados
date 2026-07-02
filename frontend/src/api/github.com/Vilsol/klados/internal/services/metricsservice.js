// @ts-nocheck
// Web-transport facade for klados.v1.MetricsService.

import { metrics, fromJsonBytes } from "$lib/transport/clients";

export function GetCapabilities(clusterCtx) {
    return metrics.getCapabilities({ context: clusterCtx }).then((r) => fromJsonBytes(r.resultJson));
}

export function GetResourceMetrics(clusterCtx, gvr, $namespace, name, rangeMinutes) {
    return metrics
        .getResourceMetrics({ context: clusterCtx, gvr, namespace: $namespace, name, rangeMinutes })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function GetNamespaceMetrics(clusterCtx, $namespace, rangeMinutes) {
    return metrics
        .getNamespaceMetrics({ context: clusterCtx, namespace: $namespace, rangeMinutes })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function GetListMetrics(clusterCtx, gvr, $namespace) {
    return metrics
        .getListMetrics({ context: clusterCtx, gvr, namespace: $namespace })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function GetPluginMetrics(clusterCtx, gvr, $namespace, name, rangeMinutes) {
    return metrics
        .getPluginMetrics({ context: clusterCtx, gvr, namespace: $namespace, name, rangeMinutes })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function SetPrometheusEndpoint(clusterCtx, url) {
    return metrics.setPrometheusEndpoint({ context: clusterCtx, url }).then(() => undefined);
}

export function RedetectSources(clusterCtx) {
    return metrics.redetectSources({ context: clusterCtx }).then((r) => fromJsonBytes(r.resultJson));
}
