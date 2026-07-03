// Drop-in replacement for @wailsio/runtime, backed by the klados server:
// Events ride Connect server-streams from the event hub; Browser/System map
// to web APIs. Wired in via a Vite alias so app code and the generated
// models.js files keep their imports unchanged.

import { Code, ConnectError } from "@connectrpc/connect";
import { app, events, fromJsonBytes, toJsonBytes } from "./clients";

export interface WailsEvent {
	name: string;
	// Matches the original @wailsio/runtime typing: payloads are dynamic and
	// call sites narrow them in their callback signatures.
	// biome-ignore lint/suspicious/noExplicitAny: dynamic event payloads by design
	data: any;
}

// biome-ignore lint/suspicious/noExplicitAny: callbacks declare narrowed payload shapes
type EventCallback = (ev: any) => void;

/**
 * All Events.On subscriptions share ONE Connect server-stream. Browsers only
 * get HTTP/2 over TLS; on plain HTTP/1.1 each held-open stream eats one of
 * the ~6 connections per origin, so per-topic streams starve every other
 * request. Instead, when the topic set changes the mux opens a replacement
 * stream carrying the full set and only tears the old one down once the new
 * one is live (the server acks with an empty first batch) — make-before-
 * break, so no watch events fall in the gap. Duplicates during the overlap
 * are harmless: watch applies are keyed upserts/deletes.
 */
class EventMux {
	private topics = new Map<string, Set<EventCallback>>();
	private current: AbortController | null = null;
	private reopenScheduled = false;
	private generation = 0;
	private backoff = 500;

	on(name: string, cb: EventCallback): () => void {
		let set = this.topics.get(name);
		if (!set) {
			set = new Set();
			this.topics.set(name, set);
			this.scheduleReopen();
		}
		set.add(cb);

		return () => {
			const callbacks = this.topics.get(name);
			if (!callbacks) return;
			callbacks.delete(cb);
			if (callbacks.size === 0) {
				this.topics.delete(name);
				this.scheduleReopen();
			}
		};
	}

	// Coalesce bursts of subscription changes (page mounts register several
	// topics in one tick) into a single stream reopen. A microtask coalesces
	// same-tick subscriptions without adding wall-clock latency — important
	// because a subscribe-then-immediately-await-reply handshake (pop-out
	// panels) must get its subscription live as fast as possible.
	private scheduleReopen() {
		if (this.reopenScheduled) return;
		this.reopenScheduled = true;
		queueMicrotask(() => {
			this.reopenScheduled = false;
			this.reopen();
		});
	}

	private reopen() {
		const gen = ++this.generation;
		const names = Array.from(this.topics.keys());

		if (names.length === 0) {
			this.current?.abort();
			this.current = null;
			return;
		}

		const ac = new AbortController();
		const previous = this.current;
		this.current = ac;

		(async () => {
			while (!ac.signal.aborted && gen === this.generation) {
				try {
					const stream = events.subscribe({ topics: names }, { signal: ac.signal });
					let acked = false;
					for await (const batch of stream) {
						this.backoff = 500;
						if (!acked) {
							// New stream is live — retire the one it replaces.
							acked = true;
							previous?.abort();
						}
						for (const ev of batch.events) {
							const callbacks = this.topics.get(ev.name);
							if (!callbacks) continue;
							const payload = { name: ev.name, data: fromJsonBytes(ev.payloadJson) };
							for (const cb of callbacks) cb(payload);
						}
					}
				} catch {
					// aborted or transport error — fall through to backoff/retry
				}
				if (ac.signal.aborted || gen !== this.generation) return;
				await new Promise((r) => setTimeout(r, this.backoff));
				this.backoff = Math.min(this.backoff * 2, 5000);
			}
		})();
	}
}

const mux = new EventMux();

export const Events = {
	On(name: string, cb: EventCallback): () => void {
		return mux.on(name, cb);
	},
	Emit(name: string, data?: unknown): Promise<void> {
		return events
			.publish({ event: { name, payloadJson: toJsonBytes(data) } })
			.then(() => undefined);
	},
};

export const Browser = {
	// Desktop opens the system default browser via the AppService RPC (a Wails
	// webview's window.open would try to navigate inside the app, not launch
	// Safari/Chrome). Server mode returns Unimplemented, and we fall back to
	// window.open — correct for a real browser tab.
	async OpenURL(url: string): Promise<void> {
		try {
			await app.openURL({ url });
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.Unimplemented) {
				window.open(url, "_blank", "noopener,noreferrer");
				return;
			}
			throw err;
		}
	},
};

export const System = {
	IsMac(): boolean {
		const platform = navigator.platform ?? "";
		return /mac/i.test(platform) || /mac os/i.test(navigator.userAgent);
	},
};

// ---------------------------------------------------------------------------
// Compatibility surface for the generated bindings/models files.
// ---------------------------------------------------------------------------

export type CancellablePromise<T> = Promise<T>;

export const Call = {
	ByID(): never {
		throw new Error(
			"Wails Call.ByID is not available in the web build — the bindings facade should route through the Connect clients instead",
		);
	},
};

// $Create helpers mirror @wailsio/runtime's marshalling combinators used by
// the generated models.js files. Payloads are already plain JSON here, so
// they reduce to structural passthroughs.
type Creator<T = unknown> = (source: unknown) => T;

function identity<T>(source: unknown): T {
	return source as T;
}

export const Create = {
	Any: identity,
	Nullable<T>(creator: Creator<T>): Creator<T | null> {
		return (source) => (source === null || source === undefined ? null : creator(source));
	},
	Array<T>(creator: Creator<T>): Creator<T[]> {
		return (source) => (Array.isArray(source) ? source.map((it) => creator(it)) : []);
	},
	Map<V>(_keyCreator: Creator, valueCreator: Creator<V>): Creator<Record<string, V>> {
		return (source) => {
			if (source === null || typeof source !== "object") return {};
			const out: Record<string, V> = {};
			for (const [k, v] of Object.entries(source)) out[k] = valueCreator(v);
			return out;
		};
	},
	// Event payload hydrator used by some generated code — passthrough.
	Events: identity,
	Struct: identity,
};
