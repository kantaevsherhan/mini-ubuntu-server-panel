<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import api from '../services/api'
import { useI18n } from '../services/i18n'
import { usePreferencesStore } from '../stores/preferences'

interface TicketResponse {
  ticket: string
  subprotocol: string
}

export type TerminalState = 'connecting' | 'connected' | 'reconnecting' | 'detached' | 'exited'

const props = defineProps<{ sessionId: string; active: boolean }>()
const emit = defineEmits<{ state: [TerminalState]; exited: [] }>()

const { t } = useI18n()
const preferences = usePreferencesStore()
const host = ref<HTMLElement>()
let shell: Terminal | undefined
let socket: WebSocket | undefined
let resizeObserver: ResizeObserver | undefined
let reconnectTimer: number | undefined
let attempts = 0
let disposed = false

function terminalTheme() {
  const styles = getComputedStyle(document.documentElement)
  return {
    background: styles.getPropertyValue('--terminal-background').trim() || '#0b0f14',
    foreground: styles.getPropertyValue('--terminal-foreground').trim() || '#e6edf3',
    cursor: styles.getPropertyValue('--p-primary-color').trim() || '#10b981',
    selectionBackground: 'rgba(16, 185, 129, 0.3)',
  }
}

function fit() {
  if (!host.value || !shell || !props.active || host.value.clientWidth === 0) return
  const columns = Math.min(300, Math.max(20, Math.floor((host.value.clientWidth - 16) / 8.4)))
  const rows = Math.min(120, Math.max(5, Math.floor((host.value.clientHeight - 8) / 18)))
  if (shell.cols !== columns || shell.rows !== rows) shell.resize(columns, rows)
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: 'resize', columns, rows }))
  }
}

function scheduleReconnect() {
  if (disposed) return
  emit('state', 'reconnecting')
  attempts++
  const delay = Math.min(15000, 500 * 2 ** Math.min(attempts, 5))
  window.clearTimeout(reconnectTimer)
  reconnectTimer = window.setTimeout(connect, delay)
}

async function connect() {
  if (disposed) return
  emit('state', attempts ? 'reconnecting' : 'connecting')
  let data: TicketResponse
  try {
    data = (await api.post<TicketResponse>('/terminal/tickets', undefined, { silent: true })).data
  } catch {
    scheduleReconnect()
    return
  }
  if (disposed) return
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const next = new WebSocket(
    `${scheme}//${location.host}/api/v1/terminal/ws?session=${encodeURIComponent(props.sessionId)}`,
    [data.subprotocol, `ticket.${data.ticket}`],
  )
  next.binaryType = 'arraybuffer'
  socket = next
  let replayed = false
  next.onopen = () => {
    attempts = 0
    emit('state', 'connected')
    fit()
    if (props.active) shell?.focus()
  }
  next.onmessage = (event) => {
    // The first frame is the server-side scrollback; start from a clean screen to avoid duplicates.
    if (!replayed) {
      replayed = true
      shell?.reset()
    }
    if (event.data instanceof ArrayBuffer) shell?.write(new Uint8Array(event.data))
    else if (typeof event.data === 'string') shell?.write(event.data)
  }
  next.onclose = (event) => {
    if (socket === next) socket = undefined
    if (disposed) return
    if (event.code === 1000 || event.code === 4404) {
      shell?.writeln(`\r\n\x1b[33m${t.value.terminalSessionEnded}\x1b[0m`)
      emit('state', 'exited')
      emit('exited')
      return
    }
    if (event.code === 4001) {
      // Another browser tab took this shell over; do not fight it, reconnect on demand.
      emit('state', 'detached')
      return
    }
    scheduleReconnect()
  }
}

function reconnect() {
  if (socket) return
  attempts = 0
  void connect()
}

watch(
  () => props.active,
  async (active) => {
    if (!active) return
    await nextTick()
    fit()
    shell?.focus()
  },
)

watch(
  () => [preferences.mode, preferences.preset, preferences.accent],
  async () => {
    await nextTick()
    if (shell) shell.options.theme = terminalTheme()
  },
)

onMounted(() => {
  shell = new Terminal({
    cursorBlink: true,
    convertEol: false,
    fontFamily: "'JetBrains Mono', 'Cascadia Code', 'Ubuntu Mono', ui-monospace, monospace",
    fontSize: 14,
    lineHeight: 1.15,
    scrollback: 5000,
    theme: terminalTheme(),
  })
  shell.open(host.value!)
  shell.onData((data) => {
    if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'input', data }))
  })
  resizeObserver = new ResizeObserver(fit)
  resizeObserver.observe(host.value!)
  void connect()
})

onBeforeUnmount(() => {
  // Leaving the page only detaches the browser; the shell keeps running on the server.
  disposed = true
  window.clearTimeout(reconnectTimer)
  socket?.close(1000, 'detached')
  resizeObserver?.disconnect()
  shell?.dispose()
})

defineExpose({ focus: () => shell?.focus(), fit, reconnect })
</script>

<template>
  <div ref="host" class="terminal-view" />
</template>

<style scoped>
.terminal-view {
  height: 100%;
  width: 100%;
  padding: 8px 4px 4px 10px;
  background: var(--terminal-background, #0b0f14);
}

:deep(.xterm) {
  height: 100%;
}

:deep(.xterm-viewport) {
  scrollbar-color: var(--p-surface-600) transparent;
}
</style>
