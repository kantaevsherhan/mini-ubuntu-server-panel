<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import MultiSelect from 'primevue/multiselect'
import Skeleton from 'primevue/skeleton'
import ToggleSwitch from 'primevue/toggleswitch'
import api from '../services/api'
import { useI18n } from '../services/i18n'

interface MonitorSettings {
  interval_seconds: number
  docker_enabled: boolean
  docker_ignore: string[]
  services_enabled: boolean
  services_watch: string[]
  resources_enabled: boolean
  cpu_percent: number
  memory_percent: number
  swap_percent: number
  disk_percent: number
}

const { t } = useI18n()
const toast = useToast()
const settings = ref<MonitorSettings>()
const containerNames = ref<string[]>([])
const unitNames = ref<string[]>([])
const saving = ref(false)

async function load() {
  const [settingsResponse, containers, units] = await Promise.allSettled([
    api.get<MonitorSettings>('/notifications/monitor'),
    api.get<Array<{ name: string }>>('/docker/containers', { silent: true }),
    api.get<Array<{ name: string }>>('/services', { silent: true }),
  ])
  if (settingsResponse.status === 'fulfilled') settings.value = settingsResponse.value.data
  const current = settings.value
  // Keep already-selected names selectable even if the object no longer exists.
  if (containers.status === 'fulfilled') {
    containerNames.value = [
      ...new Set([
        ...containers.value.data.map((item) => item.name),
        ...(current?.docker_ignore ?? []),
      ]),
    ].sort()
  }
  if (units.status === 'fulfilled') {
    unitNames.value = [
      ...new Set([
        ...units.value.data.map((item) => item.name),
        ...(current?.services_watch ?? []),
      ]),
    ].sort()
  }
}

async function save() {
  if (!settings.value) return
  saving.value = true
  try {
    settings.value = (await api.put<MonitorSettings>('/notifications/monitor', settings.value)).data
    toast.add({ severity: 'success', summary: t.value.saved, life: 3000 })
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="panel-card p-5">
    <div class="mb-1 flex flex-wrap items-center justify-between gap-3">
      <h2 class="m-0 text-lg font-semibold">{{ t.monitoring }}</h2>
      <Button
        :label="t.save"
        icon="pi pi-check"
        size="small"
        :loading="saving"
        :disabled="!settings"
        @click="save"
      />
    </div>
    <p class="muted mt-0 text-sm">{{ t.monitoringHint }}</p>

    <div v-if="!settings" class="space-y-3">
      <Skeleton height="2.5rem" />
      <Skeleton height="2.5rem" />
    </div>
    <div v-else class="monitor-grid">
      <section class="monitor-block">
        <label class="monitor-label" for="monitor-interval">{{ t.checkInterval }}</label>
        <InputNumber
          v-model="settings.interval_seconds"
          input-id="monitor-interval"
          :min="15"
          :max="3600"
          suffix=" s"
          show-buttons
          :step="15"
          fluid
        />
        <p class="muted m-0 text-xs">{{ t.checkIntervalHint }}</p>
      </section>

      <section class="monitor-block">
        <div class="monitor-toggle">
          <span><i class="pi pi-box mr-2 text-primary" />{{ t.monitorDocker }}</span>
          <ToggleSwitch v-model="settings.docker_enabled" />
        </div>
        <MultiSelect
          v-model="settings.docker_ignore"
          :options="containerNames"
          :placeholder="t.ignoredContainers"
          :disabled="!settings.docker_enabled"
          filter
          display="chip"
          fluid
        />
        <p class="muted m-0 text-xs">{{ t.monitorDockerHint }}</p>
      </section>

      <section class="monitor-block">
        <div class="monitor-toggle">
          <span><i class="pi pi-cog mr-2 text-primary" />{{ t.monitorServices }}</span>
          <ToggleSwitch v-model="settings.services_enabled" />
        </div>
        <MultiSelect
          v-model="settings.services_watch"
          :options="unitNames"
          :placeholder="t.watchedServices"
          :disabled="!settings.services_enabled"
          filter
          display="chip"
          fluid
        />
        <p class="muted m-0 text-xs">{{ t.monitorServicesHint }}</p>
      </section>

      <section class="monitor-block">
        <div class="monitor-toggle">
          <span><i class="pi pi-gauge mr-2 text-primary" />{{ t.monitorResources }}</span>
          <ToggleSwitch v-model="settings.resources_enabled" />
        </div>
        <div class="grid grid-cols-2 gap-2">
          <label
            v-for="field in [
              'cpu_percent',
              'memory_percent',
              'swap_percent',
              'disk_percent',
            ] as const"
            :key="field"
            class="flex flex-col gap-1 text-xs"
          >
            <span class="muted">{{ t[field] }}</span>
            <InputNumber
              v-model="settings[field]"
              :min="10"
              :max="100"
              suffix=" %"
              :disabled="!settings.resources_enabled"
              fluid
            />
          </label>
        </div>
        <p class="muted m-0 text-xs">{{ t.monitorResourcesHint }}</p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.monitor-grid {
  display: grid;
  gap: 14px;
  margin-top: 12px;
}

@media (min-width: 900px) {
  .monitor-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.monitor-block {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
}

.monitor-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-weight: 500;
}

.monitor-label {
  font-weight: 500;
}
</style>
