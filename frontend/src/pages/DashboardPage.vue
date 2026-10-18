<script setup lang="ts">
import { computed, ref } from 'vue'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import SelectButton from 'primevue/selectbutton'
import Skeleton from 'primevue/skeleton'
import Tag from 'primevue/tag'
import MetricsHistoryChart, { type MetricPoint } from '../components/MetricsHistoryChart.vue'
import { useAutoRefresh } from '../composables/useAutoRefresh'
import api from '../services/api'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

interface DashboardSummary {
  hostname: string
  panel_users: number
  pending_notifications: number
}

interface SystemSummary {
  os: string
  kernel: string
  cpu_count: number
  uptime_seconds: number
  load: [number, number, number]
  timezone: string
  reboot_required: boolean
  updates_summary: string
  disks: Array<{ mountpoint: string; total_bytes: number; used_bytes: number }>
}

const { t, locale } = useI18n()
const auth = useAuthStore()
const dashboard = ref<DashboardSummary>()
const system = ref<SystemSummary>()
const points = ref<MetricPoint[]>([])
const range = ref('day')
const metricsLoading = ref(true)
const canSeeSystem = computed(() => auth.role === 'admin' || auth.role === 'operator')
const rangeOptions = computed(() => [
  { label: t.value.day, value: 'day' },
  { label: t.value.week, value: 'week' },
  { label: t.value.month, value: 'month' },
  { label: t.value.allTime, value: 'all' },
])
const latest = computed(() => points.value.at(-1))
const rootDisk = computed(
  () => system.value?.disks.find((disk) => disk.mountpoint === '/') ?? system.value?.disks[0],
)

function percent(used = 0, total = 0) {
  return total ? Math.round((used / total) * 100) : 0
}

function formatUptime(seconds = 0) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const suffix = locale.value === 'ru' ? ['д', 'ч', 'м'] : ['d', 'h', 'm']
  return days
    ? `${days}${suffix[0]} ${hours}${suffix[1]}`
    : `${hours}${suffix[1]} ${minutes}${suffix[2]}`
}

function level(value: number) {
  if (value >= 90) return { bar: 'bar-danger', severity: 'danger', label: t.value.loadCritical }
  if (value >= 75) return { bar: 'bar-warn', severity: 'warn', label: t.value.loadHigh }
  return { bar: '', severity: 'success', label: t.value.loadNormal }
}

const gauges = computed(() => {
  const items = [
    {
      label: 'CPU',
      path: '',
      value: Math.round(latest.value?.cpu_percent ?? 0),
      icon: 'pi pi-microchip',
    },
    {
      label: t.value.memory,
      path: '',
      value: Math.round(latest.value?.memory_percent ?? 0),
      icon: 'pi pi-database',
    },
  ]
  if (rootDisk.value) {
    items.push({
      label: t.value.disk,
      path: rootDisk.value.mountpoint,
      value: percent(rootDisk.value.used_bytes, rootDisk.value.total_bytes),
      icon: 'pi pi-inbox',
    })
  }
  return items
})

async function loadMetrics(background = false) {
  if (!background) metricsLoading.value = true
  try {
    points.value = (
      await api.get('/metrics/history', { params: { range: range.value }, silent: background })
    ).data.points
  } finally {
    metricsLoading.value = false
  }
}

async function load(background: boolean) {
  const requests: Promise<unknown>[] = [
    api
      .get<DashboardSummary>('/dashboard', { silent: background })
      .then((response) => (dashboard.value = response.data)),
    loadMetrics(background),
  ]
  if (canSeeSystem.value) {
    requests.push(
      api
        .get<SystemSummary>('/system/info', { silent: true })
        .then((response) => (system.value = response.data)),
    )
  }
  await Promise.allSettled(requests)
}

// Metrics are sampled once a minute on the server, so a 30s poll is plenty.
useAutoRefresh(load, 30000)
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <h1 class="page-title">{{ t.welcome }}</h1>
        <p class="page-subtitle">
          <span class="font-medium text-[var(--p-text-color)]">{{
            dashboard?.hostname || 'ubuntu-server'
          }}</span>
          <template v-if="system"> · {{ system.os }} · {{ system.timezone }}</template>
        </p>
      </div>
    </header>

    <div v-if="system?.reboot_required || system?.updates_summary" class="mb-4 space-y-2">
      <Message v-if="system.reboot_required" severity="warn" :closable="false">
        <i class="pi pi-refresh mr-2" />{{ t.rebootRequired }}
      </Message>
      <Message v-if="system.updates_summary" severity="info" :closable="false">
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 whitespace-pre-line text-sm">{{ system.updates_summary }}</div>
          <RouterLink to="/updates" class="text-sm font-medium text-[var(--p-primary-color)]"
            >{{ t.openUpdates }} →</RouterLink
          >
        </div>
      </Message>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <div v-for="gauge in gauges" :key="gauge.label" class="panel-card stat-card">
        <div class="stat-card__label">
          <span class="stat-icon"><i :class="gauge.icon" /></span>{{ gauge.label
          }}<span v-if="gauge.path" class="font-mono text-xs">{{ gauge.path }}</span>
          <Tag
            :severity="level(gauge.value).severity"
            :value="level(gauge.value).label"
            class="ml-auto !text-[0.7rem]"
          />
        </div>
        <div class="stat-card__value">{{ gauge.value }}<span class="stat-card__unit">%</span></div>
        <ProgressBar
          :value="gauge.value"
          :show-value="false"
          :class="['mt-4 !h-1.5', level(gauge.value).bar]"
        />
      </div>
    </div>

    <div class="mt-4 grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div v-if="system" class="panel-card stat-card">
        <div class="stat-card__label">{{ t.uptime }}</div>
        <div class="stat-card__value">{{ formatUptime(system.uptime_seconds) }}</div>
      </div>
      <div v-if="system" class="panel-card stat-card">
        <div class="stat-card__label">Load · {{ system.cpu_count }} CPU</div>
        <div class="stat-card__value whitespace-nowrap" style="font-size: 1.2rem">
          {{ system.load.map((v) => v.toFixed(2)).join(' ') }}
        </div>
      </div>
      <div class="panel-card stat-card">
        <div class="stat-card__label">{{ t.panelUsers }}</div>
        <Skeleton v-if="!dashboard" width="3rem" height="2rem" class="mt-2" />
        <div v-else class="stat-card__value">{{ dashboard.panel_users }}</div>
      </div>
      <div class="panel-card stat-card">
        <div class="stat-card__label">{{ t.pending }}</div>
        <div class="stat-card__value">{{ dashboard?.pending_notifications ?? 0 }}</div>
      </div>
    </div>

    <div class="panel-card mt-4 p-5">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <h2 class="m-0 text-base font-semibold">{{ t.metricsHistory }}</h2>
        <SelectButton
          v-model="range"
          :options="rangeOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
          size="small"
          class="ml-auto"
          @change="loadMetrics()"
        />
      </div>
      <Skeleton v-if="metricsLoading" width="100%" height="360px" />
      <MetricsHistoryChart
        v-else-if="points.length"
        :points="points"
        :locale="locale"
        cpu-label="CPU"
        :memory-label="t.memory"
      />
      <div v-else class="muted grid h-[300px] place-items-center text-center">
        <div>
          <i class="pi pi-chart-line mb-3 text-3xl" />
          <p>{{ t.noMetrics }}</p>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.bar-warn :deep(.p-progressbar-value) {
  background: var(--app-warn);
}
.bar-danger :deep(.p-progressbar-value) {
  background: var(--app-danger);
}
.stat-card__unit {
  margin-left: 2px;
  font-size: 1rem;
  font-weight: 500;
  color: var(--p-text-muted-color);
}
</style>
