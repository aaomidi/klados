// Connect clients for every klados.v1 service, plus helpers shared by the
// bindings facade. Generated schemas live in src/gen (run `buf generate`).

import { createClient, type Client } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { serverOrigin } from "./base";
import { AppService } from "../../gen/klados/v1/app_pb";
import { ClusterService } from "../../gen/klados/v1/cluster_pb";
import { ConfigService } from "../../gen/klados/v1/config_pb";
import { DrainService } from "../../gen/klados/v1/drain_pb";
import { EventService } from "../../gen/klados/v1/event_pb";
import { ExecService } from "../../gen/klados/v1/exec_pb";
import { HelmService } from "../../gen/klados/v1/helm_pb";
import { LogService } from "../../gen/klados/v1/log_pb";
import { MetricsService } from "../../gen/klados/v1/metrics_pb";
import { PluginService } from "../../gen/klados/v1/plugin_pb";
import { PortForwardService } from "../../gen/klados/v1/portforward_pb";
import { ResourceService } from "../../gen/klados/v1/resource_pb";
import { SchemaService } from "../../gen/klados/v1/schema_pb";
import { VolumeBrowserService } from "../../gen/klados/v1/volumebrowser_pb";
import { WindowService } from "../../gen/klados/v1/window_pb";

// Binary framing keeps the JSON-bytes payloads (resource lists, watch
// batches) untouched on the wire — no base64 detour, no double encoding.
const transport = createConnectTransport({
	baseUrl: serverOrigin(),
	useBinaryFormat: true,
});

export const app: Client<typeof AppService> = createClient(AppService, transport);
export const cluster: Client<typeof ClusterService> = createClient(ClusterService, transport);
export const configSvc: Client<typeof ConfigService> = createClient(ConfigService, transport);
export const drain: Client<typeof DrainService> = createClient(DrainService, transport);
export const events: Client<typeof EventService> = createClient(EventService, transport);
export const exec: Client<typeof ExecService> = createClient(ExecService, transport);
export const helm: Client<typeof HelmService> = createClient(HelmService, transport);
export const log: Client<typeof LogService> = createClient(LogService, transport);
export const metrics: Client<typeof MetricsService> = createClient(MetricsService, transport);
export const plugin: Client<typeof PluginService> = createClient(PluginService, transport);
export const portforward: Client<typeof PortForwardService> = createClient(PortForwardService, transport);
export const resource: Client<typeof ResourceService> = createClient(ResourceService, transport);
export const schema: Client<typeof SchemaService> = createClient(SchemaService, transport);
export const volumebrowser: Client<typeof VolumeBrowserService> = createClient(VolumeBrowserService, transport);
export const window: Client<typeof WindowService> = createClient(WindowService, transport);

const decoder = new TextDecoder();
const encoder = new TextEncoder();

/**
 * Decode a `bytes *_json` response field into the value it encodes.
 * Deliberately `any`-typed: these payloads replace the old Wails bindings'
 * dynamically-shaped returns, and call sites already narrow them.
 */
// biome-ignore lint/suspicious/noExplicitAny: dynamic JSON payloads by design
export function fromJsonBytes<T = any>(data: Uint8Array): T {
	if (!data || data.length === 0) return null as T;
	return JSON.parse(decoder.decode(data)) as T;
}

/** Encode a value into a `bytes *_json` request field. */
export function toJsonBytes(value: unknown): Uint8Array {
	if (value === undefined || value === null) return new Uint8Array();
	return encoder.encode(JSON.stringify(value));
}
