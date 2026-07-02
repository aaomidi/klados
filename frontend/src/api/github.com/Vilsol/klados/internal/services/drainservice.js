// @ts-nocheck
// Web-transport facade for klados.v1.DrainService.

import { drain } from "$lib/transport/clients";

export function StartDrain(contextName, nodeName) {
    return drain.startDrain({ context: contextName, nodeName }).then(() => undefined);
}

export function CancelDrain(contextName, nodeName) {
    return drain.cancelDrain({ context: contextName, nodeName }).then(() => undefined);
}

export function IsActive(contextName, nodeName) {
    return drain.isActive({ context: contextName, nodeName }).then((r) => r.active);
}

export function ListActive(contextName) {
    return drain.listActive({ context: contextName }).then((r) => r.values);
}

export function CordonNode(contextName, nodeName) {
    return drain.cordonNode({ context: contextName, nodeName }).then(() => undefined);
}

export function UncordonNode(contextName, nodeName) {
    return drain.uncordonNode({ context: contextName, nodeName }).then(() => undefined);
}
