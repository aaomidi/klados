import {serverOrigin, httpUrl, wsUrl} from "$lib/transport/base";

export interface StreamingConfig {
  port: number;
  token: string;
  origin: string;
}

/**
 * The streaming plane is same-origin with the Connect API now — no separate
 * loopback server, no token handshake. `config` is synthesized immediately so
 * panels that gate on it render without waiting; URL helpers below are the
 * real API.
 */
class StreamingStore {
  config = $state<StreamingConfig | null>(null);

  constructor() {
    if (typeof window !== "undefined") {
      const origin = new URL(serverOrigin());
      this.config = {
        port: Number(origin.port) || (origin.protocol === "https:" ? 443 : 80),
        token: "",
        origin: serverOrigin(),
      };
    }
  }

  /** ws(s):// URL for a log stream. */
  logStreamUrl(streamId: string): string {
    return wsUrl(`/ws/logs/${streamId}`);
  }

  /** ws(s):// URL for an exec session. */
  execSessionUrl(sessionId: string): string {
    return wsUrl(`/ws/exec/${sessionId}`);
  }

  /** http(s):// base for plugin module imports (no trailing slash). */
  pluginsBaseUrl(): string {
    return httpUrl("/plugins");
  }

  /** http(s):// endpoint for console log forwarding. */
  logSinkUrl(): string {
    return httpUrl("/log");
  }
}

export const streamingStore = new StreamingStore();
