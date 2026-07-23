<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { init, use, type ECharts } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import moment from 'moment'
import type { Locale } from '../stores/preferences'

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

const option = computed(() => ({
  animation: props.points.length < 300,
  grid: { left: 45, right: 20, top: 40, bottom: 35 },
  legend: { textStyle: { color: 'var(--p-text-muted-color)' } },
  tooltip: {
    trigger: 'axis',
    valueFormatter: (value: unknown) => `${Number(value).toFixed(1)}%`,
  },
  xAxis: {
    type: 'time',
    axisLabel: {
      color: 'var(--p-text-muted-color)',
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
    axisLabel: { color: 'var(--p-text-muted-color)', formatter: '{value}%' },
    splitLine: { lineStyle: { color: 'var(--p-content-border-color)' } },
  },
  series: [
    {
      name: props.cpuLabel,
      type: 'line',
      showSymbol: false,
      smooth: true,
      data: props.points.map((point) => [point.sampled_at, point.cpu_percent]),
      lineStyle: { width: 2 },
    },
    {
      name: props.memoryLabel,
      type: 'line',
      showSymbol: false,
      smooth: true,
      data: props.points.map((point) => [point.sampled_at, point.memory_percent]),
      lineStyle: { width: 2 },
    },
  ],
}))

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
onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  chart?.dispose()
})
</script>

<template><div ref="root" class="h-[300px] w-full sm:h-[380px]" /></template>
