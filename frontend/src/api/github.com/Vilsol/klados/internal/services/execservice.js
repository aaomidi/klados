// @ts-nocheck
// Web-transport facade for klados.v1.ExecService.

import { exec } from "$lib/transport/clients";

export function OpenExecSession(ctxName, ns, podName, container, shell) {
    return exec
        .openExecSession({ context: ctxName, namespace: ns, podName, container, shell })
        .then((r) => r.sessionId);
}

export function CloseExecSession(sessionID) {
    return exec.closeExecSession({ sessionId: sessionID }).then(() => undefined);
}
