// @ts-nocheck
// Web-transport facade for klados.v1.LogService.

import { log } from "$lib/transport/clients";

export function StartLogStream(ctxName, ns, podName, opts) {
    const o = opts ?? {};
    return log
        .startLogStream({
            context: ctxName,
            namespace: ns,
            podName,
            options: {
                follow: !!o.follow,
                timestamps: !!o.timestamps,
                previous: !!o.previous,
                container: o.container ?? "",
                tailLines: o.tailLines === null || o.tailLines === undefined ? undefined : BigInt(o.tailLines),
            },
        })
        .then((r) => r.streamId);
}

export function StopLogStream(streamID) {
    return log.stopLogStream({ streamId: streamID }).then(() => undefined);
}
