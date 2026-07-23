<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

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

async function load() {
  loading.value = true
  try {
    items.value = (await api.get<SystemProcess[]>('/processes')).data
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

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold">{{ t.processes }}</h1>
        <p class="muted mt-1 text-sm">{{ t.processesHint }}</p>
      </div>
      <Button :label="t.refresh" icon="pi pi-refresh" :loading="loading" @click="load" />
    </div>

    <InputText v-model="query" :placeholder="t.searchProcesses" class="w-full sm:max-w-md" />

    <DataTable
      :value="filtered"
      :loading="loading"
      scrollable
      scroll-height="calc(100vh - 15rem)"
      :virtual-scroller-options="{ itemSize: 45, delay: 100 }"
      size="small"
      striped-rows
      data-key="pid"
      class="min-h-96"
    >
      <Column field="pid" header="PID" sortable class="w-24" />
      <Column field="name" :header="t.processName" sortable />
      <Column field="username" :header="t.username" sortable />
      <Column field="state" :header="t.status" class="w-24">
        <template #body="{ data }"
          ><Tag :value="data.state" :severity="stateSeverity(data.state)"
        /></template>
      </Column>
      <Column field="cpu_percent" header="CPU" sortable class="w-28">
        <template #body="{ data }">{{ data.cpu_percent.toFixed(1) }}%</template>
      </Column>
      <Column field="memory_bytes" :header="t.memory" sortable class="w-32">
        <template #body="{ data }">{{ formatBytes(data.memory_bytes) }}</template>
      </Column>
      <Column field="started_at" :header="t.startedAt" sortable>
        <template #body="{ data }">{{ formatDateTime(data.started_at, locale) }}</template>
      </Column>
      <Column field="command" :header="t.command" class="max-w-96">
        <template #body="{ data }"
          ><span class="block truncate font-mono text-xs" :title="data.command">{{
            data.command
          }}</span></template
        >
      </Column>
      <Column v-if="canManage" :header="t.actions" frozen align-frozen="right" class="w-40">
        <template #body="{ data }">
          <div class="flex gap-1">
            <Button label="HUP" size="small" text @click="requestSignal(data, 'HUP')" />
            <Button
              label="TERM"
              size="small"
              text
              severity="warn"
              @click="requestSignal(data, 'TERM')"
            />
            <Button
              label="KILL"
              size="small"
              text
              severity="danger"
              @click="requestSignal(data, 'KILL')"
            />
          </div>
        </template>
      </Column>
      <template #empty>{{ t.noProcesses }}</template>
    </DataTable>
  </section>
</template>
