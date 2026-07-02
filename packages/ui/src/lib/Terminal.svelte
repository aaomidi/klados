<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { WebglAddon } from '@xterm/addon-webgl'
  import { ClipboardAddon } from '@xterm/addon-clipboard'

  interface StreamingConfig { port: number; token: string; origin?: string }

  let { sessionID, streamingConfig, ondisconnect, useWebGL, onSetShortcutMode, fontSize = 13, onclear }: {
    sessionID: string
    streamingConfig: StreamingConfig
    ondisconnect?: () => void
    useWebGL?: boolean
    onSetShortcutMode?: (mode: string) => void
    fontSize?: number
    onclear?: (fn: () => void) => void
  } = $props()

  let container: HTMLDivElement

  let readyFlag = $state(false)
  let termRef: Terminal | null = null
  let fitAddonRef: FitAddon | null = null
  let sendResizeRef: (() => void) | null = null

  $effect(() => {
    const size = fontSize
    if (!readyFlag) return
    const t = untrack(() => termRef)
    const fa = untrack(() => fitAddonRef)
    const notify = untrack(() => sendResizeRef)
    if (!t || !fa) return
    t.options.fontSize = size
    requestAnimationFrame(() => { fa.fit(); notify?.() })
  })

  function buildURL() {
    const origin = streamingConfig.origin ?? window.location.origin
    return `${origin.replace(/^http/, 'ws')}/ws/exec/${sessionID}`
  }

  onMount(() => {
    const term = new Terminal({
      scrollback: 10000,
      fontFamily: 'monospace',
      fontSize,
      cursorBlink: true,
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)

    term.textarea?.addEventListener('focus', () => onSetShortcutMode?.('terminal'))
    term.textarea?.addEventListener('blur', () => onSetShortcutMode?.('normal'))

    if (useWebGL ?? false) {
      try {
        term.loadAddon(new WebglAddon())
      } catch {
        // unsupported
      }
    }

    try {
      term.loadAddon(new ClipboardAddon())
    } catch {
      // optional
    }

    termRef = term
    fitAddonRef = fitAddon
    onclear?.(() => term.clear())
    readyFlag = true

    const ws = new WebSocket(buildURL())
    ws.binaryType = 'arraybuffer'

    function sendResize() {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }

    sendResizeRef = sendResize
    ws.onopen = () => sendResize()

    ws.onmessage = (e) => {
      if (e.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(e.data))
      }
    }

    ws.onclose = () => {
      term.writeln('\r\n[session closed]')
      ondisconnect?.()
    }

    ws.onerror = () => term.writeln('\r\n[connection error]')

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })

    const ro = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect
      if (width === 0 || height === 0) return
      if (!term.element) {
        term.open(container)
      }
      fitAddon.fit()
      sendResize()
    })
    ro.observe(container)

    return () => {
      readyFlag = false
      termRef = null
      fitAddonRef = null
      sendResizeRef = null
      ro.disconnect()
      ws.close()
      term.dispose()
    }
  })
</script>

<div bind:this={container} class="h-full w-full bg-[#1a1a1a] overflow-hidden"></div>
