<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'

interface AuditEvent {
  id: number
  actor_user_id: number | null
  action: string
  target_type: string
  target_id: string | null
  details: string
  ip_address: string
  created_at: string
}

const events = ref<AuditEvent[]>([])
const query = ref('')
const loading = ref(false)
const selected = ref<AuditEvent | null>(null)
const detailsVisible = ref(false)
const { t, locale } = useI18n()

const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return events.value
  return events.value.filter((event) =>
    [event.action, event.target_type, event.target_id || '', event.ip_address, event.details].some(
      (field) => field.toLocaleLowerCase().includes(value),
    ),
  )
})

async function load() {
  loading.value = true
  try {
    events.value = (await api.get<AuditEvent[]>('/audit')).data
  } finally {
    loading.value = false
  }
}

function showDetails(event: AuditEvent) {
  selected.value = event
  detailsVisible.value = true
}

onMounted(load)
</script>

<template>
  <section>
    <div class="mb-4 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div>
        <h1 class="m-0 text-2xl font-semibold">{{ t.audit }}</h1>
        <p class="muted mb-0 mt-2">{{ t.auditHint }}</p>
      </div>
      <div class="flex w-full gap-2 md:w-auto">
        <InputText v-model="query" :placeholder="t.searchAudit" class="min-w-0 flex-1 md:w-80" />
        <Button :label="t.refresh" icon="pi pi-refresh" :loading="loading" @click="load" />
      </div>
    </div>

    <DataTable
      :value="filtered"
      :loading="loading"
      size="small"
      scrollable
      scroll-height="calc(100vh - 13rem)"
      :virtual-scroller-options="{ itemSize: 49 }"
      striped-rows
      class="panel-card"
    >
      <template #empty>{{ t.noAuditEvents }}</template>
      <Column field="created_at" :header="t.time" style="min-width: 11rem">
        <template #body="{ data }">{{ formatDateTime(data.created_at, locale) }}</template>
      </Column>
      <Column field="action" :header="t.action" style="min-width: 13rem">
        <template #body="{ data }"><Tag severity="secondary" :value="data.action" /></template>
      </Column>
      <Column :header="t.target" style="min-width: 12rem">
        <template #body="{ data }">
          <span>{{ data.target_type }}</span>
          <span v-if="data.target_id" class="muted ml-1">· {{ data.target_id }}</span>
        </template>
      </Column>
      <Column field="actor_user_id" :header="t.auditActor" style="min-width: 7rem">
        <template #body="{ data }">{{ data.actor_user_id ?? '—' }}</template>
      </Column>
      <Column field="ip_address" :header="t.ipAddress" style="min-width: 9rem" />
      <Column frozen align-frozen="right" style="width: 4rem">
        <template #body="{ data }">
          <Button
            icon="pi pi-eye"
            text
            rounded
            :aria-label="t.viewDetails"
            @click="showDetails(data)"
          />
        </template>
      </Column>
    </DataTable>

    <Dialog
      v-model:visible="detailsVisible"
      modal
      :header="selected?.action || t.details"
      :style="{ width: 'min(44rem, calc(100vw - 2rem))' }"
    >
      <dl v-if="selected" class="m-0 grid gap-3 sm:grid-cols-[9rem_1fr]">
        <dt class="muted">{{ t.time }}</dt>
        <dd class="m-0">{{ formatDateTime(selected.created_at, locale) }}</dd>
        <dt class="muted">{{ t.target }}</dt>
        <dd class="m-0">{{ selected.target_type }} · {{ selected.target_id || '—' }}</dd>
        <dt class="muted">{{ t.auditActor }}</dt>
        <dd class="m-0">{{ selected.actor_user_id ?? '—' }}</dd>
        <dt class="muted">{{ t.ipAddress }}</dt>
        <dd class="m-0">{{ selected.ip_address || '—' }}</dd>
        <dt class="muted">{{ t.details }}</dt>
        <dd class="m-0 break-all font-mono text-sm">{{ selected.details || '{}' }}</dd>
      </dl>
    </Dialog>
  </section>
</template>
