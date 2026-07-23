<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import api from '../services/api'
import { useI18n } from '../services/i18n'
import { usePreferencesStore } from '../stores/preferences'

interface TicketResponse {
  ticket: string
  expires_at: string
  subprotocol: string
}

const { t } = useI18n()
const preferences = usePreferencesStore()
const toast = useToast()
const host = ref<HTMLElement>()
const shell = ref<Terminal>()
const socket = ref<WebSocket>()
const connected = ref(false)
const connecting = ref(false)
const fullscreen = ref(localStorage.getItem('terminal-fullscreen') === 'true')
const manualClose = ref(false)
const panelHeight = ref(
  Math.min(900, Math.max(360, Number(localStorage.getItem('terminal-height') || 560))),
)
let resizeObserver: ResizeObserver | undefined
let inputSubscription: { dispose(): void } | undefined

const statusSeverity = computed(() =>
  connected.value ? 'success' : connecting.value ? 'warn' : 'secondary',
)
const statusLabel = computed(() =>
  connected.value
    ? t.value.terminalConnected
    : connecting.value
      ? t.value.terminalConnecting
      : t.value.terminalDisconnected,
)

function terminalTheme() {
  const styles = getComputedStyle(document.documentElement)
  return {
    background: styles.getPropertyValue('--p-content-background').trim() || '#09090b',
    foreground: styles.getPropertyValue('--p-text-color').trim() || '#f4f4f5',
    cursor: styles.getPropertyValue('--p-primary-color').trim() || '#10b981',
    selectionBackground: styles.getPropertyValue('--p-highlight-background').trim() || '#064e3b',
  }
}

function resizeTerminal() {
  if (!host.value || !shell.value) return
  const columns = Math.min(300, Math.max(20, Math.floor((host.value.clientWidth - 24) / 8.4)))
  const rows = Math.min(120, Math.max(5, Math.floor((host.value.clientHeight - 16) / 18)))
  if (shell.value.cols !== columns || shell.value.rows !== rows) shell.value.resize(columns, rows)
  if (socket.value?.readyState === WebSocket.OPEN) {
    socket.value.send(JSON.stringify({ type: 'resize', columns, rows }))
  }
  if (!fullscreen.value && host.value.clientHeight >= 360) {
    panelHeight.value = Math.min(900, host.value.clientHeight)
    localStorage.setItem('terminal-height', String(panelHeight.value))
  }
}

async function connect() {
  if (connecting.value || connected.value) return
  connecting.value = true
  manualClose.value = false
  try {
    const { data } = await api.post<TicketResponse>('/terminal/tickets')
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const nextSocket = new WebSocket(`${scheme}//${location.host}/api/v1/terminal/ws`, [
      data.subprotocol,
      `ticket.${data.ticket}`,
    ])
    nextSocket.binaryType = 'arraybuffer'
    socket.value = nextSocket
    nextSocket.onopen = () => {
      connecting.value = false
      connected.value = true
      shell.value?.clear()
      shell.value?.writeln(`\x1b[32m${t.value.terminalSessionStarted}\x1b[0m`)
      resizeTerminal()
      shell.value?.focus()
    }
    nextSocket.onmessage = (event) => {
      if (typeof event.data === 'string') shell.value?.write(event.data)
      else if (event.data instanceof ArrayBuffer) shell.value?.write(new Uint8Array(event.data))
    }
    nextSocket.onerror = () => {
      if (!manualClose.value) {
        toast.add({ severity: 'error', summary: t.value.terminalConnectionFailed, life: 5000 })
      }
    }
    nextSocket.onclose = () => {
      connected.value = false
      connecting.value = false
      socket.value = undefined
      shell.value?.writeln(`\r\n\x1b[33m${t.value.terminalSessionEnded}\x1b[0m`)
    }
  } catch {
    connecting.value = false
  }
}

function disconnect() {
  manualClose.value = true
  socket.value?.close(1000, 'user disconnected')
}

async function toggleFullscreen() {
  fullscreen.value = !fullscreen.value
  localStorage.setItem('terminal-fullscreen', String(fullscreen.value))
  await nextTick()
  resizeTerminal()
  shell.value?.focus()
}

onMounted(() => {
  shell.value = new Terminal({
    cursorBlink: true,
    convertEol: true,
    fontFamily: "'JetBrains Mono', 'Ubuntu Mono', ui-monospace, monospace",
    fontSize: 14,
    scrollback: 5000,
    theme: terminalTheme(),
  })
  shell.value.open(host.value!)
  inputSubscription = shell.value.onData((data) => {
    if (socket.value?.readyState === WebSocket.OPEN)
      socket.value.send(JSON.stringify({ type: 'input', data }))
  })
  resizeObserver = new ResizeObserver(resizeTerminal)
  resizeObserver.observe(host.value!)
  resizeTerminal()
  void connect()
})

watch(
  () => [preferences.mode, preferences.preset, preferences.accent],
  async () => {
    await nextTick()
    if (shell.value) shell.value.options.theme = terminalTheme()
  },
)

onBeforeUnmount(() => {
  manualClose.value = true
  socket.value?.close(1000, 'page closed')
  resizeObserver?.disconnect()
  inputSubscription?.dispose()
  shell.value?.dispose()
})
</script>

<template>
  <section :class="['terminal-workspace', { 'terminal-workspace--fullscreen': fullscreen }]">
    <header class="mb-3 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="m-0 text-2xl font-semibold">{{ t.terminal }}</h1>
        <p class="muted mb-0 mt-1 text-sm">{{ t.terminalSecurityHint }}</p>
      </div>
      <div class="flex items-center gap-2">
        <Tag :severity="statusSeverity" :value="statusLabel" />
        <Button
          v-if="!connected && !connecting"
          :label="t.connect"
          icon="pi pi-play"
          size="small"
          @click="connect"
        />
        <Button
          v-else
          :label="t.disconnect"
          icon="pi pi-stop"
          severity="danger"
          outlined
          size="small"
          :disabled="connecting"
          @click="disconnect"
        />
        <Button
          :icon="fullscreen ? 'pi pi-window-minimize' : 'pi pi-window-maximize'"
          text
          rounded
          :aria-label="fullscreen ? t.exitFullscreen : t.fullscreen"
          @click="toggleFullscreen"
        />
      </div>
    </header>

    <Message severity="info" :closable="false" class="mb-3">
      {{ t.terminalPrivilegeHint }}
    </Message>
    <div
      ref="host"
      class="terminal-host panel-card"
      :style="fullscreen ? undefined : { height: `${panelHeight}px` }"
      :aria-label="t.terminal"
    />
    <p v-if="!fullscreen" class="muted mt-2 text-xs">
      <i class="pi pi-arrows-v mr-1" />{{ t.terminalResizeHint }}
    </p>
  </section>
</template>

<style scoped>
.terminal-host {
  min-height: 360px;
  max-height: 900px;
  overflow: hidden;
  resize: vertical;
  padding: 8px;
  background: var(--p-content-background);
}

.terminal-workspace--fullscreen {
  position: fixed;
  inset: 0;
  z-index: 1100;
  display: flex;
  flex-direction: column;
  padding: 16px;
  background: var(--p-surface-950);
}

.terminal-workspace--fullscreen .terminal-host {
  min-height: 0;
  max-height: none;
  flex: 1;
  resize: none;
}

:deep(.xterm) {
  height: 100%;
}

:deep(.xterm-viewport) {
  scrollbar-color: var(--p-surface-600) transparent;
}

:global(html[data-theme='light']) .terminal-workspace--fullscreen {
  background: var(--p-surface-50);
}
</style>
