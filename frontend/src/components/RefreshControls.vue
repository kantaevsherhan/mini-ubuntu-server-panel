<script setup lang="ts">
import Button from 'primevue/button'
import { useI18n } from '../services/i18n'

defineProps<{ loading: boolean; auto: boolean }>()
defineEmits<{ refresh: []; toggle: [] }>()
const { t } = useI18n()
</script>

<template>
  <div class="refresh-controls">
    <Button
      v-tooltip.bottom="auto ? t.autoRefreshOn : t.autoRefreshOff"
      :icon="auto ? 'pi pi-pause' : 'pi pi-play'"
      severity="secondary"
      text
      rounded
      :aria-label="auto ? t.autoRefreshOn : t.autoRefreshOff"
      @click="$emit('toggle')"
    />
    <Button
      :label="t.refresh"
      icon="pi pi-refresh"
      severity="secondary"
      outlined
      :loading="loading"
      @click="$emit('refresh')"
    />
  </div>
</template>

<style scoped>
.refresh-controls {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}
</style>
