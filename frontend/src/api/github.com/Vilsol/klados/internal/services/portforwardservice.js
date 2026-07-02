// @ts-nocheck
// Web-transport facade for klados.v1.PortForwardService.

import { portforward, fromJsonBytes, toJsonBytes } from "$lib/transport/clients";

export function StartForward(contextName, $namespace, targetKind, targetName, targetGVR, localPort, remotePort) {
    return portforward
        .startForward({
            context: contextName,
            namespace: $namespace,
            targetKind,
            targetName,
            targetGvr: targetGVR,
            localPort,
            remotePort,
        })
        .then((r) => fromJsonBytes(r.resultJson));
}

export function StopForward(forwardID) {
    return portforward.stopForward({ forwardId: forwardID }).then(() => undefined);
}

export function ConnectSavedForward(ctxName, savedID) {
    return portforward.connectSavedForward({ context: ctxName, savedId: savedID }).then((r) => fromJsonBytes(r.resultJson));
}

export function ListForwards(contextName) {
    return portforward.listForwards({ context: contextName }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}

export function SavePortForward(ctxName, fwd) {
    return portforward.savePortForward({ context: ctxName, forwardJson: toJsonBytes(fwd) }).then(() => undefined);
}

export function RemoveSavedPortForward(ctxName, id) {
    return portforward.removeSavedPortForward({ context: ctxName, savedId: id }).then(() => undefined);
}

export function SetPortForwardEnabled(ctxName, id, enabled) {
    return portforward.setPortForwardEnabled({ context: ctxName, id, enabled }).then(() => undefined);
}

export function ListSavedPortForwards(ctxName) {
    return portforward.listSavedPortForwards({ context: ctxName }).then((r) => fromJsonBytes(r.resultJson) ?? []);
}
