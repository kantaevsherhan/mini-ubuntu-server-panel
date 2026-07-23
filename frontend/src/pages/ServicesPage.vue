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
import { useI18n } from '../services/i18n'

interface SystemService {
  name: string
  load_state: string
  active_state: string
  sub_state: string
  enabled: string
  description: string
}

type ServiceAction = 'start' | 'stop' | 'restart' | 'enable' | 'disable'

const items = ref<SystemService[]>([])
const query = ref('')
const loading = ref(false)
const busyUnit = ref('')
const confirm = useConfirm()
const toast = useToast()
const { t } = useI18n()
const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return items.value
  return items.value.filter((item) =>
    [item.name, item.description, item.active_state, item.enabled].some((field) =>
      field.toLocaleLowerCase().includes(value),
    ),
  )
})

async function load() {
  loading.value = true
  try {
    items.value = (await api.get<SystemService[]>('/services')).data
  } finally {
    loading.value = false
  }
}

function activeSeverity(state: string) {
  if (state === 'active') return 'success'
  if (state === 'failed') return 'danger'
  if (state === 'activating' || state === 'deactivating') return 'warn'
  return 'secondary'
}

function enabledSeverity(state: string) {
  if (state === 'enabled') return 'info'
  if (state === 'masked') return 'danger'
  return 'secondary'
}

function requestAction(service: SystemService, action: ServiceAction) {
  confirm.require({
    header: t.value.serviceAction,
    message: `${t.value.serviceActionConfirm}: ${service.name} — ${t.value[action]}`,
    icon: action === 'stop' || action === 'disable' ? 'pi pi-exclamation-triangle' : 'pi pi-cog',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.confirm,
    accept: async () => {
      busyUnit.value = service.name
      try {
        await api.post(`/services/${encodeURIComponent(service.name)}/action`, { action })
        toast.add({ severity: 'success', summary: t.value.serviceActionDone, life: 3000 })
        await load()
      } finally {
        busyUnit.value = ''
      }
    },
  })
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold">{{ t.services }}</h1>
        <p class="muted mt-1 text-sm">{{ t.servicesHint }}</p>
      </div>
      <Button :label="t.refresh" icon="pi pi-refresh" :loading="loading" @click="load" />
    </div>

    <InputText v-model="query" :placeholder="t.searchServices" class="w-full sm:max-w-md" />

    <DataTable
      :value="filtered"
      :loading="loading"
      scrollable
      scroll-height="calc(100vh - 15rem)"
      :virtual-scroller-options="{ itemSize: 48, delay: 100 }"
      size="small"
      striped-rows
      data-key="name"
      class="min-h-96"
    >
      <Column field="name" :header="t.serviceName" sortable>
        <template #body="{ data }"
          ><span class="font-mono text-xs">{{ data.name }}</span></template
        >
      </Column>
      <Column field="description" :header="t.description" sortable />
      <Column field="active_state" :header="t.status" sortable class="w-32">
        <template #body="{ data }"
          ><Tag :value="data.active_state" :severity="activeSeverity(data.active_state)"
        /></template>
      </Column>
      <Column field="sub_state" :header="t.serviceSubState" sortable class="w-28" />
      <Column field="enabled" :header="t.autostart" sortable class="w-32">
        <template #body="{ data }"
          ><Tag :value="data.enabled || '—'" :severity="enabledSeverity(data.enabled)"
        /></template>
      </Column>
      <Column :header="t.actions" frozen align-frozen="right" class="w-72">
        <template #body="{ data }">
          <Tag
            v-if="data.name === 'mini-ubuntu-server.service'"
            :value="t.protectedService"
            severity="secondary"
            icon="pi pi-lock"
          />
          <div v-else class="flex gap-1">
            <Button
              icon="pi pi-play"
              size="small"
              text
              severity="success"
              :aria-label="t.start"
              :loading="busyUnit === data.name"
              @click="requestAction(data, 'start')"
            />
            <Button
              icon="pi pi-stop"
              size="small"
              text
              severity="danger"
              :aria-label="t.stop"
              @click="requestAction(data, 'stop')"
            />
            <Button
              icon="pi pi-refresh"
              size="small"
              text
              :aria-label="t.restart"
              @click="requestAction(data, 'restart')"
            />
            <Button
              v-if="data.enabled !== 'enabled'"
              :label="t.enable"
              size="small"
              text
              @click="requestAction(data, 'enable')"
            />
            <Button
              v-else
              :label="t.disable"
              size="small"
              text
              severity="warn"
              @click="requestAction(data, 'disable')"
            />
          </div>
        </template>
      </Column>
      <template #empty>{{ t.noServices }}</template>
    </DataTable>
  </section>
</template>
