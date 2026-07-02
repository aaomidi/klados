// Base URL resolution for the klados server. Same-origin by default (the Go
// server serves the SPA); overridable for dev where Vite serves the frontend
// and the server runs elsewhere.

const override = import.meta.env.VITE_KLADOS_SERVER as string | undefined;

/** HTTP(S) origin of the klados server, no trailing slash. */
export function serverOrigin(): string {
	if (override) return override.replace(/\/$/, "");
	return window.location.origin;
}

/** Absolute HTTP URL for a server path ("/log", "/plugins/..."). */
export function httpUrl(path: string): string {
	return serverOrigin() + path;
}

/** Absolute WebSocket URL for a server path ("/ws/logs/{id}"). */
export function wsUrl(path: string): string {
	return serverOrigin().replace(/^http/, "ws") + path;
}
