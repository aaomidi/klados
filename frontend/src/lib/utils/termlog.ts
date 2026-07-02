import {streamingStore} from "$lib/stores/streaming.svelte.js";

export function termlog(msg: string): void {
  if (!streamingStore.config) {
    return;
  }
  fetch(streamingStore.logSinkUrl(), {
    method: "POST",
    body: msg,
  }).catch(() => {
    /* best-effort */
  });
}
