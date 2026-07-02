<script lang="ts">
  import { untrack } from 'svelte'
  import VirtualLogViewer from './VirtualLogViewer.svelte'

  interface StreamingConfig { port: number; token: string; origin?: string }

  let { streamID, streamingConfig, showTimestamps = false, filename = 'logs', scrollToTopOnLoad = false, fontSize = 13 }: {
    streamID: string
    streamingConfig: StreamingConfig
    showTimestamps?: boolean
    filename?: string
    scrollToTopOnLoad?: boolean
    fontSize?: number
  } = $props()

  let lines: string[] = $state([])
  let eofReached = $state(false)
  let eofHistory = $state(false)
  let lineBuffer = ''
  let viewer = $state<ReturnType<typeof VirtualLogViewer>>()

  export function downloadVisible() { viewer?.downloadVisible() }
  export function scrollToTop() { viewer?.scrollToTop() }

  $effect(() => {
    if (!initialLoading && scrollToTopOnLoad) {
      untrack(() => viewer?.scrollToTop())
    }
  })

  let ws: WebSocket | null = null
  let historyLoading = $state(false)
  let lastHistoryLoad = 0
  let initialLoading = $state(true)
  let settleTimer: ReturnType<typeof setTimeout> | null = null

  function scheduleReady() {
    if (!initialLoading) return
    if (settleTimer) clearTimeout(settleTimer)
    settleTimer = setTimeout(() => {
      settleTimer = null
      initialLoading = false
    }, 150)
  }

  function setReady() {
    if (settleTimer) { clearTimeout(settleTimer); settleTimer = null }
    initialLoading = false
  }

  function buildURL(sid: string) {
    const origin = streamingConfig.origin ?? window.location.origin
    return `${origin.replace(/^http/, 'ws')}/ws/logs/${sid}`
  }

  function ingestChunk(chunk: string) {
    const parts = (lineBuffer + chunk).split('\n')
    lineBuffer = parts.pop() ?? ''
    if (parts.length > 0) lines.push(...parts)
    scheduleReady()
  }

  export function loadHistory() {
    const now = Date.now()
    if (historyLoading || eofHistory || !ws || ws.readyState !== WebSocket.OPEN) return
    if (now - lastHistoryLoad < 500) return
    lastHistoryLoad = now
    historyLoading = true
    ws.send(JSON.stringify({ type: 'load_history', count: 200, alreadyHave: lines.length }))
  }

  $effect(() => {
    const sid = streamID
    if (!sid) return

    lines = []
    lineBuffer = ''
    eofReached = false
    eofHistory = false
    historyLoading = false
    lastHistoryLoad = 0
    initialLoading = true
    if (settleTimer) { clearTimeout(settleTimer); settleTimer = null }

    const socket = new WebSocket(buildURL(sid))
    ws = socket

    socket.onmessage = (e) => {
      if (typeof e.data !== 'string') return
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'eof') { eofReached = true; setReady(); return }
        if (msg.type === 'error') { lines.push(`[error: ${msg.message}]`); setReady(); return }
        if (msg.type === 'history') {
          const batch: string[] = msg.lines ?? []
          viewer?.prependLines(batch)
          lines = [...batch, ...lines]
          historyLoading = false
          if (!msg.has_more) eofHistory = true
          return
        }
      } catch {
        // not JSON — raw log line
      }
      ingestChunk(e.data)
    }
    socket.onerror = () => { lines.push('[connection error]'); setReady() }

    return () => {
      ws = null
      socket.close()
      if (settleTimer) { clearTimeout(settleTimer); settleTimer = null }
    }
  })
</script>

{#if initialLoading}
  <div class="flex items-center justify-center h-full text-sm text-muted">Loading…</div>
{:else}
  <VirtualLogViewer
    bind:this={viewer}
    {lines}
    {eofReached}
    {eofHistory}
    {historyLoading}
    {showTimestamps}
    {filename}
    {fontSize}
    onLoadHistory={() => loadHistory()}
  />
{/if}
