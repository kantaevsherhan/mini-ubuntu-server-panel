<script setup lang="ts">
import { computed } from 'vue'
import { cellText, type Cell } from '../../services/databaseClient'

const props = defineProps<{ value: Cell | undefined }>()
const isNull = computed(() => props.value === null || props.value === undefined)
const text = computed(() => cellText(props.value))
const display = computed(() =>
  text.value.length > 300 ? text.value.slice(0, 300) + '…' : text.value,
)
</script>

<template>
  <span v-if="isNull" class="db-null">NULL</span>
  <span
    v-else
    :class="['db-cell', { 'db-cell--number': typeof value === 'number' }]"
    :title="text.length > 40 ? text.slice(0, 2000) : undefined"
    >{{ display }}</span
  >
</template>

<style scoped>
.db-cell {
  display: block;
  max-width: 28rem;
  overflow: hidden;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.78rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.db-cell--number {
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.db-null {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.72rem;
  font-style: italic;
  color: var(--p-text-muted-color);
}
</style>
