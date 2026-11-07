<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { init, use, type ECharts } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import moment from 'moment'
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
const preferences = usePreferencesStore()
const narrow = ref(false)
let chart: ECharts | undefined
let resizeObserver: ResizeObserver | undefined

// ECharts draws on canvas, which cannot resolve CSS custom properties, so theme
// tokens are read once per render and passed as concrete colors.
function token(name: string, fallback: string) {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value || fallback
}

// Canvas does not understand color-mix() either: resolve to rgb() via the DOM.
function withAlpha(color: string, alpha: number) {
  const probe = document.createElement('span')
  probe.style.color = color
  document.body.appendChild(probe)
  const resolved = getComputedStyle(probe).color
  probe.remove()
  const match = resolved.match(/[\d.]+/g)
  if (!match || match.length < 3) return color
  return `rgba(${match[0]}, ${match[1]}, ${match[2]}, ${alpha})`
}

function area(color: string) {
  return {
    type: 'linear',
    x: 0,
    y: 0,
    x2: 0,
    y2: 1,
    colorStops: [
      { offset: 0, color: withAlpha(color, 0.22) },
      { offset: 1, color: withAlpha(color, 0) },
    ],
  }
}

function buildOption() {
  const muted = token('--p-text-muted-color', '#94a3b8')
  const text = token('--p-text-color', '#e2e8f0')
  const border = token('--app-border', '#1f2530')
  const surface = token('--app-surface', '#12161d')
  const cpu = token('--p-primary-400', '#34d399')
  const memory = token('--p-sky-400', '#38bdf8')
  return {
    animation: props.points.length < 300,
    color: [cpu, memory],
    grid: { left: 44, right: 16, top: 40, bottom: 28, containLabel: false },
    legend: {
      top: 0,
      right: 0,
      icon: 'roundRect',
      itemWidth: 12,
      itemHeight: 4,
      textStyle: { color: muted },
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: surface,
      borderColor: border,
      textStyle: { color: text },
      valueFormatter: (value: unknown) => `${Number(value).toFixed(1)}%`,
    },
    xAxis: {
      type: 'time',
      splitNumber: narrow.value ? 3 : 7,
      axisLine: { lineStyle: { color: border } },
      axisTick: { show: false },
      axisLabel: {
        color: muted,
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
      axisLabel: { color: muted, formatter: '{value}%' },
      splitLine: { lineStyle: { color: border, type: 'dashed' } },
    },
    series: [
      {
        name: props.cpuLabel,
        type: 'line',
        showSymbol: false,
        smooth: true,
        data: props.points.map((point) => [point.sampled_at, point.cpu_percent]),
        lineStyle: { width: 2 },
        areaStyle: { color: area(cpu) },
      },
      {
        name: props.memoryLabel,
        type: 'line',
        showSymbol: false,
        smooth: true,
        data: props.points.map((point) => [point.sampled_at, point.memory_percent]),
        lineStyle: { width: 2 },
        areaStyle: { color: area(memory) },
      },
    ],
  }
}

async function render() {
  await nextTick()
  if (!root.value) return
  chart ??= init(root.value)
  chart.setOption(buildOption(), true)
}

onMounted(() => {
  render()
  if (root.value) {
    resizeObserver = new ResizeObserver(() => {
      const isNarrow = (root.value?.clientWidth ?? 0) < 560
      if (isNarrow !== narrow.value) {
        narrow.value = isNarrow
        render()
      }
      chart?.resize()
    })
    resizeObserver.observe(root.value)
  }
})
watch(() => [props.points, props.locale, props.cpuLabel, props.memoryLabel], render)
// Theme tokens are applied to :root asynchronously by PrimeVue.
watch(
  () => [preferences.mode, preferences.preset, preferences.accent],
  () => setTimeout(render, 50),
)
onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  chart?.dispose()
})
</script>

<template><div ref="root" class="h-[300px] w-full sm:h-[380px]" /></template>
