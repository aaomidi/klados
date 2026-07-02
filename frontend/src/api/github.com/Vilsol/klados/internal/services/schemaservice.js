// @ts-nocheck
// Web-transport facade for klados.v1.SchemaService.

import { schema, fromJsonBytes } from "$lib/transport/clients";

export function GetSchema(contextName, gvr, kind) {
    return schema.getSchema({ context: contextName, gvr, kind }).then((r) => fromJsonBytes(r.resultJson));
}
