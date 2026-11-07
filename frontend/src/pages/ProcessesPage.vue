<script setup lang="ts">
import { computed, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import RefreshControls from '../components/RefreshControls.vue'
import { useAutoRefresh } from '../composables/useAutoRefresh'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'
import { useTableRow } from '../composables/useTableRow'

interface SystemProcess {
  pid: number
  name: string
  username: string
  state: string
  cpu_percent: number
  memory_bytes: number
  command: string
  started_at: string
}

const items = ref<SystemProcess[]>([])
const query = ref('')
const loading = ref(false)
const confirm = useConfirm()
const toast = useToast()
const auth = useAuthStore()
const { t, locale } = useI18n()
const { rowSize } = useTableRow()
const canManage = computed(() => ['admin', 'operator'].includes(auth.role))
const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return items.value
  return items.value.filter((item) =>
    [item.pid, item.name, item.username, item.command].some((field) =>
      String(field).toLocaleLowerCase().includes(value),
    ),
  )
})

async function load(background = false) {
  if (!background) loading.value = true
  try {
    items.value = (await api.get<SystemProcess[]>('/processes', { silent: background })).data
  } finally {
    loading.value = false
  }
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toLocaleString(locale.value, { maximumFractionDigits: 1 })} ${units[index]}`
}

function stateSeverity(state: string) {
  if (state === 'R') return 'success'
  if (state === 'Z' || state === 'X') return 'danger'
  if (state === 'D') return 'warn'
  return 'secondary'
}

function requestSignal(process: SystemProcess, signal: 'TERM' | 'KILL' | 'HUP') {
  confirm.require({
    header: t.value.processAction,
    message: `${t.value.processSignalConfirm}: ${process.name} (PID ${process.pid}) — ${signal}`,
    icon: signal === 'KILL' ? 'pi pi-exclamation-triangle' : 'pi pi-info-circle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.confirm,
    acceptClass: signal === 'KILL' ? 'p-button-danger' : undefined,
    accept: async () => {
      await api.post(`/processes/${process.pid}/signal`, { signal })
      toast.add({ severity: 'success', summary: t.value.processSignalSent, life: 3000 })
      await load()
    },
  })
}

const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh(load, 5000)
</script>

<template>
  <section class="page">
    <header class="page-header">
      <div>
        <h1 class="page-title">{{ t.processes }}</h1>
        <p class="page-subtitle">{{ t.processesHint }}</p>
      </div>
      <RefreshControls
        :loading="loading"
        :auto="autoRefresh"
        @refresh="load()"
        @toggle="toggleAutoRefresh"
      />
    </header>

    <InputText v-model="query" :placeholder="t.searchProcesses" class="w-full sm:max-w-md" />

    <DataTable
      :value="filtered"
      :loading="loading"
      scrollable
      scroll-height="calc(100vh - 15rem)"
      :virtual-scroller-options="{ itemSize: rowSize, delay: 100 }"
      size="small"
      striped-rows
      data-key="pid"
      class="vt min-h-96"
    >
      <Column field="pid" header="PID" sortable>
        <template #body="{ data }"
          ><span class="tabular-nums">{{ data.pid }}</span></template
        >
      </Column>
      <Column field="name" :header="t.processName" sortable>
        <template #body="{ data }"
          ><span class="cell-clip font-medium [--clip:12rem]" :title="data.name">{{
            data.name
          }}</span></template
        >
      </Column>
      <Column field="username" :header="t.username" sortable>
        <template #body="{ data }"
          ><span class="cell-clip [--clip:10rem]" :title="data.username">{{
            data.username
          }}</span></template
        >
      </Column>
      <Column field="state" :header="t.status" class="w-24">
        <template #body="{ data }"
          ><Tag :value="data.state" :severity="stateSeverity(data.state)"
        /></template>
      </Column>
      <Column field="cpu_percent" header="CPU" sortable class="w-28">
        <template #body="{ data }"
          ><span class="tabular-nums">{{ data.cpu_percent.toFixed(1) }}%</span></template
        >
      </Column>
      <Column field="memory_bytes" :header="t.memory" sortable class="w-32">
        <template #body="{ data }"
          ><span class="tabular-nums">{{ formatBytes(data.memory_bytes) }}</span></template
        >
      </Column>
      <Column field="started_at" :header="t.startedAt" sortable>
        <template #body="{ data }"
          ><span class="muted">{{ formatDateTime(data.started_at, locale) }}</span></template
        >
      </Column>
      <Column field="command" :header="t.command" class="w-full">
        <template #body="{ data }"
          ><span class="cell-clip font-mono text-xs [--clip:28rem]" :title="data.command">{{
            data.command
          }}</span></template
        >
      </Column>
      <Column v-if="canManage" :header="t.actions" frozen align-frozen="right">
        <template #body="{ data }">
          <div class="flex justify-end gap-1">
            <Button
              v-tooltip.top="'SIGHUP'"
              icon="pi pi-replay"
              size="small"
              text
              rounded
              severity="secondary"
              aria-label="HUP"
              @click="requestSignal(data, 'HUP')"
            />
            <Button
              v-tooltip.top="'SIGTERM'"
              icon="pi pi-stop-circle"
              size="small"
              text
              rounded
              severity="warn"
              aria-label="TERM"
              @click="requestSignal(data, 'TERM')"
            />
            <Button
              v-tooltip.top="'SIGKILL'"
              icon="pi pi-times-circle"
              size="small"
              text
              rounded
              severity="danger"
              aria-label="KILL"
              @click="requestSignal(data, 'KILL')"
            />
          </div>
        </template>
      </Column>
      <template #empty>{{ t.noProcesses }}</template>
    </DataTable>
  </section>
</template>
