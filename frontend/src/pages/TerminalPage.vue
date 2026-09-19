<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import TerminalView, { type TerminalState } from '../components/TerminalView.vue'
import api from '../services/api'
import { useI18n } from '../services/i18n'

interface TerminalSession {
  id: string
  title: string
  created_at: string
  attached: boolean
}

const { t } = useI18n()
const confirm = useConfirm()
const sessions = ref<TerminalSession[]>([])
const activeID = ref(localStorage.getItem('terminal-active') || '')
const states = reactive<Record<string, TerminalState>>({})
const views = reactive<Record<string, InstanceType<typeof TerminalView> | null>>({})
const loading = ref(true)
const creating = ref(false)
const editingID = ref('')
const editingTitle = ref('')
const fullscreen = ref(localStorage.getItem('terminal-fullscreen') === 'true')

const activeState = computed(() => states[activeID.value])

function select(id: string) {
  activeID.value = id
  localStorage.setItem('terminal-active', id)
}

async function load() {
  loading.value = true
  try {
    sessions.value = (await api.get<TerminalSession[]>('/terminal/sessions')).data
    if (!sessions.value.length) await create()
    else if (!sessions.value.some((session) => session.id === activeID.value)) {
      select(sessions.value[0].id)
    }
  } finally {
    loading.value = false
  }
}

async function create() {
  creating.value = true
  try {
    const title = `bash ${sessions.value.length + 1}`
    const session = (await api.post<TerminalSession>('/terminal/sessions', { title })).data
    sessions.value.push(session)
    select(session.id)
  } finally {
    creating.value = false
  }
}

function removeLocal(id: string) {
  const index = sessions.value.findIndex((session) => session.id === id)
  if (index < 0) return
  sessions.value.splice(index, 1)
  delete states[id]
  if (activeID.value === id) {
    const next = sessions.value[Math.max(0, index - 1)]
    if (next) select(next.id)
    else activeID.value = ''
  }
}

function close(session: TerminalSession) {
  confirm.require({
    header: t.value.closeTerminal,
    message: `${t.value.closeTerminalConfirm}: ${session.title}`,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.closeTerminal,
    acceptClass: 'p-button-danger',
    accept: async () => {
      await api.delete(`/terminal/sessions/${session.id}`).catch(() => undefined)
      removeLocal(session.id)
    },
  })
}

function startRename(session: TerminalSession) {
  editingID.value = session.id
  editingTitle.value = session.title
}

async function saveRename(session: TerminalSession) {
  const title = editingTitle.value.trim()
  editingID.value = ''
  if (!title || title === session.title) return
  await api.patch(`/terminal/sessions/${session.id}`, { title })
  session.title = title
}

function stateIcon(state?: TerminalState) {
  if (state === 'connected') return 'dot dot--ok'
  if (state === 'exited') return 'dot dot--off'
  if (state === 'detached') return 'dot dot--idle'
  return 'dot dot--warn'
}

async function toggleFullscreen() {
  fullscreen.value = !fullscreen.value
  localStorage.setItem('terminal-fullscreen', String(fullscreen.value))
  await nextTick()
  views[activeID.value]?.fit()
  views[activeID.value]?.focus()
}

onMounted(load)
</script>

<template>
  <section :class="['terminal-page', { 'terminal-page--fullscreen': fullscreen }]">
    <header v-if="!fullscreen" class="page-header">
      <div>
        <h1 class="page-title">{{ t.terminal }}</h1>
        <p class="page-subtitle">{{ t.terminalPersistentHint }}</p>
      </div>
    </header>

    <div class="terminal-shell panel-card">
      <div class="terminal-tabs" role="tablist">
        <div
          v-for="session in sessions"
          :key="session.id"
          role="tab"
          :aria-selected="session.id === activeID"
          :class="['terminal-tab', { 'terminal-tab--active': session.id === activeID }]"
          @click="select(session.id)"
          @dblclick="startRename(session)"
        >
          <span :class="stateIcon(states[session.id])" />
          <InputText
            v-if="editingID === session.id"
            v-model="editingTitle"
            size="small"
            class="terminal-tab__input"
            autofocus
            @keydown.enter="saveRename(session)"
            @keydown.esc="editingID = ''"
            @blur="saveRename(session)"
            @click.stop
          />
          <span v-else class="terminal-tab__title">{{ session.title }}</span>
          <button
            type="button"
            class="terminal-tab__close"
            :aria-label="t.closeTerminal"
            @click.stop="close(session)"
          >
            <i class="pi pi-times" />
          </button>
        </div>
        <Button
          v-tooltip.bottom="t.newTerminal"
          icon="pi pi-plus"
          text
          rounded
          size="small"
          :loading="creating"
          :disabled="sessions.length >= 8"
          :aria-label="t.newTerminal"
          @click="create"
        />
        <span class="flex-1" />
        <Button
          v-if="activeState === 'detached'"
          :label="t.reconnect"
          icon="pi pi-link"
          size="small"
          text
          @click="views[activeID]?.reconnect()"
        />
        <Button
          v-tooltip.bottom="fullscreen ? t.exitFullscreen : t.fullscreen"
          :icon="fullscreen ? 'pi pi-window-minimize' : 'pi pi-window-maximize'"
          text
          rounded
          size="small"
          severity="secondary"
          :aria-label="fullscreen ? t.exitFullscreen : t.fullscreen"
          @click="toggleFullscreen"
        />
      </div>

      <div class="terminal-body">
        <div v-if="loading" class="terminal-empty"><i class="pi pi-spin pi-spinner" /></div>
        <div v-else-if="!sessions.length" class="terminal-empty">
          <Button :label="t.newTerminal" icon="pi pi-plus" @click="create" />
        </div>
        <TerminalView
          v-for="session in sessions"
          v-show="session.id === activeID"
          :key="session.id"
          :ref="(view) => (views[session.id] = view as InstanceType<typeof TerminalView> | null)"
          :session-id="session.id"
          :active="session.id === activeID"
          @state="(state) => (states[session.id] = state)"
          @exited="removeLocal(session.id)"
        />
      </div>
    </div>
    <p v-if="!fullscreen" class="muted mt-2 text-xs">
      <i class="pi pi-info-circle mr-1" />{{ t.terminalPrivilegeHint }}
    </p>
  </section>
</template>

<style scoped>
.terminal-shell {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 13rem);
  min-height: 360px;
  overflow: hidden;
}

.terminal-page--fullscreen {
  position: fixed;
  inset: 0;
  z-index: 1100;
  padding: 10px;
  background: var(--app-background);
}

.terminal-page--fullscreen .terminal-shell {
  height: 100%;
}

.terminal-tabs {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 6px 8px 0;
  border-bottom: 1px solid var(--p-content-border-color);
  overflow-x: auto;
}

.terminal-tab {
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 14rem;
  padding: 7px 8px 7px 12px;
  border: 1px solid transparent;
  border-bottom: none;
  border-radius: 8px 8px 0 0;
  font-size: 0.85rem;
  color: var(--p-text-muted-color);
  cursor: pointer;
  user-select: none;
  white-space: nowrap;
}

.terminal-tab:hover {
  color: var(--p-text-color);
  background: var(--p-content-hover-background);
}

.terminal-tab--active {
  color: var(--p-text-color);
  background: var(--terminal-background, #0b0f14);
  border-color: var(--p-content-border-color);
}

.terminal-tab__title {
  overflow: hidden;
  text-overflow: ellipsis;
}

.terminal-tab__input {
  width: 8rem;
}

.terminal-tab__close {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  opacity: 0.6;
}

.terminal-tab__close:hover {
  opacity: 1;
  background: var(--p-content-hover-background);
}

.terminal-tab__close .pi {
  font-size: 0.7rem;
}

.terminal-body {
  position: relative;
  flex: 1;
  min-height: 0;
  background: var(--terminal-background, #0b0f14);
}

.terminal-empty {
  display: grid;
  place-items: center;
  height: 100%;
  color: var(--p-text-muted-color);
}

.dot {
  width: 7px;
  height: 7px;
  flex-shrink: 0;
  border-radius: 999px;
}
.dot--ok {
  background: #22c55e;
  box-shadow: 0 0 6px #22c55e88;
}
.dot--warn {
  background: #f59e0b;
}
.dot--idle {
  background: #64748b;
}
.dot--off {
  background: #ef4444;
}
</style>
