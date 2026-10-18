<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { color as colorUtil, init, use, type ECharts } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import moment from 'moment'
import { SANS_FONT } from '../services/fonts'
import { usePreferencesStore, type Locale } from '../stores/preferences'

export interface MetricPoint {
  sampled_at: string
  cpu_percent: number
  memory_percent: number
  memory_used_bytes: number
  memory_total_bytes: number
}

const props = defineProps<{
  points: MetricPoint[]
  locale: Locale
  cpuLabel: string
  memoryLabel: string
}>()

use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])
const root = ref<HTMLDivElement>()
let chart: ECharts | undefined
let resizeObserver: ResizeObserver | undefined

const preferences = usePreferencesStore()
// Bumped on theme changes: canvas cannot resolve CSS variables, so colors are read eagerly.
const themeVersion = ref(0)

function token(name: string, fallback: string) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
}

function area(color: string) {
  return {
    type: 'linear',
    x: 0,
    y: 0,
    x2: 0,
    y2: 1,
    colorStops: [
      { offset: 0, color: colorUtil.modifyAlpha(color, 0.28) },
      { offset: 1, color: colorUtil.modifyAlpha(color, 0) },
    ],
  }
}

const option = computed(() => {
  void themeVersion.value
  const muted = token('--p-text-muted-color', '#71717a')
  const text = token('--p-text-color', '#e4e4e7')
  const border = token('--app-border', '#27272a')
  const surface = token('--app-surface', '#111114')
  const cpuColor = token('--p-primary-500', '#10b981')
  const memoryColor = token('--p-sky-500', '#0ea5e9')
  const series = [
    { name: props.cpuLabel, color: cpuColor, key: 'cpu_percent' as const },
    { name: props.memoryLabel, color: memoryColor, key: 'memory_percent' as const },
  ]
  return {
    animation: props.points.length < 300,
    textStyle: { fontFamily: SANS_FONT },
    color: series.map((entry) => entry.color),
    grid: { left: 8, right: 12, top: 40, bottom: 4, containLabel: true },
    legend: {
      top: 0,
      left: 0,
      icon: 'roundRect',
      itemWidth: 10,
      itemHeight: 10,
      itemGap: 18,
      textStyle: { color: muted, fontSize: 12 },
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: surface,
      borderColor: border,
      textStyle: { color: text, fontSize: 12 },
      axisPointer: { lineStyle: { color: muted, type: 'dashed' } },
      valueFormatter: (value: unknown) => `${Number(value).toFixed(1)}%`,
    },
    xAxis: {
      type: 'time',
      axisLine: { lineStyle: { color: border } },
      axisTick: { show: false },
      axisLabel: {
        color: muted,
        fontSize: 11,
        hideOverlap: true,
        formatter: (value: number) =>
          moment(value)
            .locale(props.locale)
            .format(props.locale === 'ru' ? 'DD.MM HH:mm' : 'MM/DD h A'),
      },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 100,
      interval: 25,
      axisLabel: { color: muted, fontSize: 11, formatter: '{value}%' },
      splitLine: { lineStyle: { color: border, type: 'dashed' } },
    },
    series: series.map((entry) => ({
      name: entry.name,
      type: 'line',
      showSymbol: props.points.length < 2,
      symbolSize: 6,
      smooth: 0.25,
      data: props.points.map((point) => [point.sampled_at, point[entry.key]]),
      lineStyle: { width: 2, color: entry.color },
      areaStyle: { color: area(entry.color) },
    })),
  }
})

async function render() {
  await nextTick()
  if (!root.value) return
  chart ??= init(root.value)
  chart.setOption(option.value, true)
}

onMounted(() => {
  render()
  if (root.value) {
    resizeObserver = new ResizeObserver(() => chart?.resize())
    resizeObserver.observe(root.value)
  }
})
watch(option, render, { deep: true })
watch(
  () => [preferences.mode, preferences.preset, preferences.accent],
  async () => {
    // Wait for PrimeVue to swap its theme stylesheet before sampling tokens.
    await nextTick()
    requestAnimationFrame(() => themeVersion.value++)
  },
)
onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  chart?.dispose()
})
</script>

<template><div ref="root" class="h-[300px] w-full sm:h-[380px]" /></template>
