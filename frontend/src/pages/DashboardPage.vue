<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Card from 'primevue/card'
import ProgressBar from 'primevue/progressbar'
import SelectButton from 'primevue/selectbutton'
import Skeleton from 'primevue/skeleton'
import MetricsHistoryChart, { type MetricPoint } from '../components/MetricsHistoryChart.vue'
import api from '../services/api'
import { useI18n } from '../services/i18n'

const { t, locale } = useI18n()
const dashboard = ref<Record<string, unknown>>()
const points = ref<MetricPoint[]>([])
const range = ref('day')
const metricsLoading = ref(true)
const rangeOptions = computed(() => [
  { label: t.value.day, value: 'day' },
  { label: t.value.week, value: 'week' },
  { label: t.value.month, value: 'month' },
  { label: t.value.allTime, value: 'all' },
])
const latest = computed(() => points.value.at(-1))
const cards = computed(() => [
  { label: 'CPU', value: Math.round(latest.value?.cpu_percent ?? 0), icon: 'pi pi-microchip' },
  {
    label: t.value.memory,
    value: Math.round(latest.value?.memory_percent ?? 0),
    icon: 'pi pi-database',
  },
])

async function loadMetrics() {
  metricsLoading.value = true
  try {
    points.value = (
      await api.get('/metrics/history', { params: { range: range.value } })
    ).data.points
  } finally {
    metricsLoading.value = false
  }
}

onMounted(async () => {
  const [dashboardResponse] = await Promise.all([api.get('/dashboard'), loadMetrics()])
  dashboard.value = dashboardResponse.data
})
</script>

<template>
  <section>
    <div class="mb-5 flex items-center">
      <div>
        <h1 class="m-0 text-2xl font-semibold">{{ t.welcome }}</h1>
        <p class="muted mt-1">{{ dashboard?.hostname || 'ubuntu-server' }}</p>
      </div>
      <span class="flex-1" />
      <span class="text-sm text-green-500">
        <i class="pi pi-circle-fill mr-2 text-[8px]" />{{ t.online }}
      </span>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <Card v-for="card in cards" :key="card.label">
        <template #content>
          <div class="flex">
            <i :class="card.icon" class="text-primary" />
            <span class="ml-2 font-medium">{{ card.label }}</span>
            <b class="ml-auto">{{ card.value }}%</b>
          </div>
          <ProgressBar :value="card.value" :show-value="false" class="mt-4 h-2" />
        </template>
      </Card>
    </div>

    <Card class="mt-4">
      <template #title>
        <div class="flex flex-wrap items-center gap-3">
          <span>{{ t.metricsHistory }}</span>
          <SelectButton
            v-model="range"
            :options="rangeOptions"
            option-label="label"
            option-value="value"
            :allow-empty="false"
            class="ml-auto"
            @change="loadMetrics"
          />
        </div>
      </template>
      <template #content>
        <Skeleton v-if="metricsLoading" width="100%" height="380px" />
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
      </template>
    </Card>

    <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
      <Card>
        <template #content>
          <div class="muted text-sm">{{ t.panelUsers }}</div>
          <Skeleton v-if="!dashboard" width="4rem" height="2rem" class="mt-3" />
          <div v-else class="mt-2 text-3xl">{{ dashboard.panel_users }}</div>
        </template>
      </Card>
      <Card>
        <template #content>
          <div class="muted text-sm">{{ t.pending }}</div>
          <div class="mt-2 text-3xl">{{ dashboard?.pending_notifications ?? 0 }}</div>
        </template>
      </Card>
    </div>
  </section>
</template>
