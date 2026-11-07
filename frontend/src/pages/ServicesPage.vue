<script setup lang="ts">
import { computed, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import RefreshControls from '../components/RefreshControls.vue'
import { useAutoRefresh } from '../composables/useAutoRefresh'
import api from '../services/api'
import { useI18n } from '../services/i18n'
import { useTableRow } from '../composables/useTableRow'

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
const { rowSize } = useTableRow()
const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return items.value
  return items.value.filter((item) =>
    [item.name, item.description, item.active_state, item.enabled].some((field) =>
      field.toLocaleLowerCase().includes(value),
    ),
  )
})

async function load(background = false) {
  if (!background) loading.value = true
  try {
    items.value = (await api.get<SystemService[]>('/services', { silent: background })).data
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

const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh(load, 15000)
</script>

<template>
  <section class="page">
    <header class="page-header">
      <div>
        <h1 class="page-title">{{ t.services }}</h1>
        <p class="page-subtitle">{{ t.servicesHint }}</p>
      </div>
      <RefreshControls
        :loading="loading"
        :auto="autoRefresh"
        @refresh="load()"
        @toggle="toggleAutoRefresh"
      />
    </header>

    <InputText v-model="query" :placeholder="t.searchServices" class="w-full sm:max-w-md" />

    <DataTable
      :value="filtered"
      :loading="loading"
      scrollable
      scroll-height="calc(100vh - 15rem)"
      :virtual-scroller-options="{ itemSize: rowSize, delay: 100 }"
      size="small"
      striped-rows
      data-key="name"
      class="vt min-h-96"
    >
      <Column field="name" :header="t.serviceName" sortable>
        <template #body="{ data }"
          ><span class="cell-clip font-mono text-xs [--clip:18rem]" :title="data.name">{{
            data.name
          }}</span></template
        >
      </Column>
      <Column field="description" :header="t.description" sortable class="w-full">
        <template #body="{ data }"
          ><span class="cell-clip muted [--clip:26rem]" :title="data.description">{{
            data.description === data.name ? '' : data.description
          }}</span></template
        >
      </Column>
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
      <Column :header="t.actions" frozen align-frozen="right">
        <template #body="{ data }">
          <Tag
            v-if="data.name === 'mini-ubuntu-server.service'"
            :value="t.protectedService"
            severity="secondary"
            icon="pi pi-lock"
          />
          <div v-else class="flex items-center justify-end gap-1">
            <Button
              v-tooltip.top="t.start"
              icon="pi pi-play"
              size="small"
              text
              severity="success"
              :aria-label="t.start"
              :loading="busyUnit === data.name"
              @click="requestAction(data, 'start')"
            />
            <Button
              v-tooltip.top="t.stop"
              icon="pi pi-stop"
              size="small"
              text
              severity="danger"
              :aria-label="t.stop"
              @click="requestAction(data, 'stop')"
            />
            <Button
              v-tooltip.top="t.restart"
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
              class="w-28"
              @click="requestAction(data, 'enable')"
            />
            <Button
              v-else
              :label="t.disable"
              size="small"
              text
              class="w-28"
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
