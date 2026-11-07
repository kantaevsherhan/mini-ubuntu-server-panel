<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
import { useTableRow } from '../composables/useTableRow'

interface LogEntry {
  timestamp: string
  unit: string
  priority: string
  message: string
  identifier: string
  pid: string
}

interface SystemService {
  name: string
}

const items = ref<LogEntry[]>([])
const services = ref<SystemService[]>([])
const query = ref('')
const selectedUnit = ref('')
const selectedPriority = ref('info')
const selectedRange = ref('day')
const selectedLimit = ref(1000)
const loading = ref(false)
const { t, locale } = useI18n()
const { rowSize } = useTableRow()

const unitOptions = computed(() => [
  { label: t.value.allServices, value: '' },
  ...services.value.map((service) => ({ label: service.name, value: service.name })),
])
const priorityOptions = computed(() => [
  { label: t.value.logError, value: 'err' },
  { label: t.value.logWarning, value: 'warning' },
  { label: t.value.logNotice, value: 'notice' },
  { label: t.value.logInfo, value: 'info' },
  { label: t.value.logDebug, value: 'debug' },
])
const rangeOptions = computed(() => [
  { label: t.value.lastHour, value: 'hour' },
  { label: t.value.lastDay, value: 'day' },
  { label: t.value.lastWeek, value: 'week' },
])
const limitOptions = [250, 500, 1000, 2000]
const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return items.value
  return items.value.filter((item) =>
    [item.unit, item.identifier, item.pid, item.message].some((field) =>
      field.toLocaleLowerCase().includes(value),
    ),
  )
})

async function loadServices() {
  try {
    services.value = (await api.get<SystemService[]>('/services')).data
  } catch {
    services.value = []
  }
}

async function load() {
  loading.value = true
  try {
    items.value = (
      await api.get<LogEntry[]>('/logs', {
        params: {
          unit: selectedUnit.value || undefined,
          priority: selectedPriority.value,
          range: selectedRange.value,
          limit: selectedLimit.value,
        },
      })
    ).data
  } finally {
    loading.value = false
  }
}

function priorityLabel(priority: string) {
  const labels: Record<string, string> = {
    '0': 'EMERG',
    '1': 'ALERT',
    '2': 'CRIT',
    '3': 'ERR',
    '4': 'WARNING',
    '5': 'NOTICE',
    '6': 'INFO',
    '7': 'DEBUG',
  }
  return labels[priority] || priority
}

function prioritySeverity(priority: string) {
  const value = Number(priority)
  if (value <= 3) return 'danger'
  if (value === 4) return 'warn'
  if (value === 5) return 'info'
  return 'secondary'
}

onMounted(async () => {
  await Promise.all([loadServices(), load()])
})
</script>

<template>
  <section class="page">
    <header class="page-header">
      <div>
        <h1 class="page-title">{{ t.logs }}</h1>
        <p class="page-subtitle">{{ t.logsHint }}</p>
      </div>
      <div class="page-actions">
        <Button
          :label="t.refresh"
          icon="pi pi-refresh"
          severity="secondary"
          outlined
          :loading="loading"
          @click="load"
        />
      </div>
    </header>

    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-5">
      <Select
        v-model="selectedUnit"
        :options="unitOptions"
        option-label="label"
        option-value="value"
        filter
        :placeholder="t.serviceName"
        @change="load"
      />
      <Select
        v-model="selectedPriority"
        :options="priorityOptions"
        option-label="label"
        option-value="value"
        @change="load"
      />
      <Select
        v-model="selectedRange"
        :options="rangeOptions"
        option-label="label"
        option-value="value"
        @change="load"
      />
      <Select v-model="selectedLimit" :options="limitOptions" @change="load" />
      <InputText v-model="query" :placeholder="t.searchLogs" />
    </div>

    <DataTable
      :value="filtered"
      :loading="loading"
      scrollable
      scroll-height="calc(100vh - 16rem)"
      :virtual-scroller-options="{ itemSize: rowSize, delay: 100 }"
      size="small"
      striped-rows
      class="vt min-h-96"
    >
      <Column field="timestamp" :header="t.time" sortable>
        <template #body="{ data }"
          ><span class="muted tabular-nums">{{
            formatDateTime(data.timestamp, locale)
          }}</span></template
        >
      </Column>
      <Column field="priority" :header="t.severity" sortable class="w-28">
        <template #body="{ data }"
          ><Tag :value="priorityLabel(data.priority)" :severity="prioritySeverity(data.priority)"
        /></template>
      </Column>
      <Column field="unit" :header="t.serviceName" sortable>
        <template #body="{ data }"
          ><span class="cell-clip font-mono text-xs [--clip:14rem]" :title="data.unit">{{
            data.unit || '—'
          }}</span></template
        >
      </Column>
      <Column field="identifier" :header="t.logSource" sortable>
        <template #body="{ data }"
          ><span class="cell-clip [--clip:12rem]"
            >{{ data.identifier }}<span v-if="data.pid" class="muted">[{{ data.pid }}]</span></span
          ></template
        >
      </Column>
      <Column field="message" :header="t.message" class="w-full">
        <template #body="{ data }"
          ><span class="cell-clip font-mono text-xs [--clip:60rem]" :title="data.message">{{
            data.message
          }}</span></template
        >
      </Column>
      <template #empty>{{ t.noLogs }}</template>
    </DataTable>
  </section>
</template>
