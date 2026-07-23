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
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'

interface DockerContainer {
  id: string
  name: string
  image: string
  state: string
  status: string
  health: string
  ports: string[]
  created_at: string
}

type ContainerAction = 'start' | 'stop' | 'restart' | 'remove'

const items = ref<DockerContainer[]>([])
const query = ref('')
const loading = ref(false)
const busyID = ref('')
const confirm = useConfirm()
const toast = useToast()
const { t, locale } = useI18n()
const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return items.value
  return items.value.filter((item) =>
    [item.name, item.image, item.state, item.status, item.id].some((field) =>
      field.toLocaleLowerCase().includes(value),
    ),
  )
})

async function load() {
  loading.value = true
  try {
    items.value = (await api.get<DockerContainer[]>('/docker/containers')).data
  } finally {
    loading.value = false
  }
}

function stateSeverity(state: string) {
  if (state === 'running') return 'success'
  if (state === 'paused' || state === 'restarting') return 'warn'
  if (state === 'dead') return 'danger'
  return 'secondary'
}

function healthSeverity(health: string) {
  if (health === 'healthy') return 'success'
  if (health === 'unhealthy') return 'danger'
  if (health === 'starting') return 'warn'
  return 'secondary'
}

function requestAction(container: DockerContainer, action: ContainerAction) {
  confirm.require({
    header: t.value.containerAction,
    message: `${t.value.containerActionConfirm}: ${container.name} — ${t.value[action]}`,
    icon: action === 'remove' || action === 'stop' ? 'pi pi-exclamation-triangle' : 'pi pi-box',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.confirm,
    acceptClass: action === 'remove' ? 'p-button-danger' : undefined,
    accept: async () => {
      busyID.value = container.id
      try {
        await api.post(`/docker/containers/${container.id}/action`, { action })
        toast.add({ severity: 'success', summary: t.value.containerActionDone, life: 3000 })
        await load()
      } finally {
        busyID.value = ''
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
        <h1 class="text-2xl font-semibold">{{ t.docker }}</h1>
        <p class="muted mt-1 text-sm">{{ t.dockerHint }}</p>
      </div>
      <Button :label="t.refresh" icon="pi pi-refresh" :loading="loading" @click="load" />
    </div>

    <InputText v-model="query" :placeholder="t.searchContainers" class="w-full sm:max-w-md" />

    <DataTable
      :value="filtered"
      :loading="loading"
      scrollable
      scroll-height="calc(100vh - 15rem)"
      :virtual-scroller-options="{ itemSize: 52, delay: 100 }"
      size="small"
      striped-rows
      data-key="id"
      class="min-h-96"
    >
      <Column field="name" :header="t.containerName" sortable />
      <Column field="image" :header="t.image" sortable />
      <Column field="state" :header="t.status" sortable class="w-28">
        <template #body="{ data }"
          ><Tag :value="data.state" :severity="stateSeverity(data.state)"
        /></template>
      </Column>
      <Column field="health" :header="t.health" sortable class="w-28">
        <template #body="{ data }"
          ><Tag
            v-if="data.health"
            :value="data.health"
            :severity="healthSeverity(data.health)"
          /><span v-else>—</span></template
        >
      </Column>
      <Column field="ports" :header="t.ports">
        <template #body="{ data }"
          ><span class="font-mono text-xs">{{ data.ports.join(', ') || '—' }}</span></template
        >
      </Column>
      <Column field="created_at" :header="t.created" sortable>
        <template #body="{ data }">{{ formatDateTime(data.created_at, locale) }}</template>
      </Column>
      <Column :header="t.actions" frozen align-frozen="right" class="w-52">
        <template #body="{ data }">
          <div class="flex gap-1">
            <Button
              v-if="data.state !== 'running'"
              icon="pi pi-play"
              size="small"
              text
              severity="success"
              :aria-label="t.start"
              :loading="busyID === data.id"
              @click="requestAction(data, 'start')"
            />
            <Button
              v-if="data.state === 'running'"
              icon="pi pi-stop"
              size="small"
              text
              severity="danger"
              :aria-label="t.stop"
              :loading="busyID === data.id"
              @click="requestAction(data, 'stop')"
            />
            <Button
              v-if="data.state === 'running'"
              icon="pi pi-refresh"
              size="small"
              text
              :aria-label="t.restart"
              :loading="busyID === data.id"
              @click="requestAction(data, 'restart')"
            />
            <Button
              v-if="data.state !== 'running'"
              icon="pi pi-trash"
              size="small"
              text
              severity="danger"
              :aria-label="t.remove"
              :loading="busyID === data.id"
              @click="requestAction(data, 'remove')"
            />
          </div>
        </template>
      </Column>
      <template #empty>{{ t.noContainers }}</template>
    </DataTable>
  </section>
</template>
