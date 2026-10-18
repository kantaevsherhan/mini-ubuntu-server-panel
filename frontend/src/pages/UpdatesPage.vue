<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { isAxiosError } from 'axios'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Skeleton from 'primevue/skeleton'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { useAutoRefresh } from '../composables/useAutoRefresh'
import api from '../services/api'
import { formatDateTime, formatRelativeTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

interface AptPackage {
  name: string
  current: string
  candidate: string
  origin: string
  security: boolean
  new: boolean
}

interface AptJob {
  state: 'idle' | 'running' | 'succeeded' | 'failed'
  operation?: 'refresh' | 'upgrade'
  started_at?: string
  finished_at?: string
  exit_code?: number
  log: string[]
}

interface AptStatus {
  packages: AptPackage[]
  security_count: number
  last_refresh?: string
  reboot_required: boolean
  reboot_packages: string[]
  auto_upgrades: { installed: boolean; enabled: boolean }
  job: AptJob
}

const JOB_POLL_MS = 2000
const { t, locale } = useI18n()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()
const status = ref<AptStatus>()
const job = ref<AptJob>({ state: 'idle', log: [] })
const selected = ref<AptPackage[]>([])
const starting = ref(false)
const logHost = ref<HTMLElement>()
let jobTimer: number | undefined

const isAdmin = computed(() => auth.role === 'admin')
const running = computed(() => job.value.state === 'running')
const packages = computed(() => status.value?.packages ?? [])
const jobLabel = computed(
  () =>
    ({
      running: t.value.aptJobRunning,
      succeeded: t.value.aptJobSucceeded,
      failed: t.value.aptJobFailed,
      idle: '',
    })[job.value.state],
)
const jobSeverity = computed(
  () =>
    ({ running: 'info', succeeded: 'success', failed: 'danger', idle: 'secondary' })[
      job.value.state
    ],
)
const autoLabel = computed(() => {
  const auto = status.value?.auto_upgrades
  if (!auto?.installed) return t.value.aptAutoMissing
  return auto.enabled ? t.value.aptAutoOn : t.value.aptAutoOff
})

async function load(background = false) {
  const { data } = await api.get<AptStatus>('/apt', { silent: background })
  status.value = data
  job.value = data.job
  const names = new Set(data.packages.map((pkg) => pkg.name))
  selected.value = selected.value.filter((pkg) => names.has(pkg.name))
}

async function pollJob() {
  window.clearTimeout(jobTimer)
  try {
    const { data } = await api.get<AptJob>('/apt/job', { silent: true })
    const finished = running.value && data.state !== 'running'
    job.value = data
    if (finished) {
      toast.add({
        severity: data.state === 'succeeded' ? 'success' : 'error',
        summary: data.state === 'succeeded' ? t.value.aptJobSucceeded : t.value.aptJobFailed,
        detail: data.operation === 'refresh' ? t.value.aptOpRefresh : t.value.aptOpUpgrade,
        life: 5000,
      })
      await load(true)
    }
  } finally {
    if (running.value) jobTimer = window.setTimeout(pollJob, JOB_POLL_MS)
  }
}

watch(running, (value) => {
  if (value) jobTimer = window.setTimeout(pollJob, JOB_POLL_MS)
})

watch(
  () => job.value.log.length,
  async () => {
    await nextTick()
    if (logHost.value) logHost.value.scrollTop = logHost.value.scrollHeight
  },
)

async function start(path: string, body?: { packages: string[] }) {
  starting.value = true
  try {
    await api.post(path, body, { silent: true })
    toast.add({ severity: 'info', summary: t.value.aptStarted, life: 3000 })
    job.value = {
      state: 'running',
      operation: path.endsWith('refresh') ? 'refresh' : 'upgrade',
      started_at: new Date().toISOString(),
      log: [],
    }
  } catch (failure) {
    const busy = isAxiosError(failure) && failure.response?.data?.error === 'apt_job_running'
    toast.add({
      severity: 'error',
      summary: busy ? t.value.aptBusy : t.value.requestError,
      detail: busy
        ? undefined
        : `${t.value.errorCode}: ${isAxiosError(failure) ? (failure.response?.data?.error ?? 'network_error') : 'unknown'}`,
      life: 6000,
    })
  } finally {
    starting.value = false
  }
}

function refresh() {
  void start('/apt/refresh')
}

function upgrade(all: boolean) {
  const names = all ? [] : selected.value.map((pkg) => pkg.name)
  const count = all ? packages.value.length : names.length
  confirm.require({
    header: all ? t.value.aptUpgradeAll : t.value.aptUpgradeSelected.replace('{n}', String(count)),
    message: (all ? t.value.aptConfirmAll : t.value.aptConfirmSelected).replace(
      '{n}',
      String(count),
    ),
    icon: 'pi pi-download',
    rejectProps: { label: t.value.cancel, severity: 'secondary', text: true },
    acceptProps: { label: t.value.confirm },
    accept: () => start('/apt/upgrade', all ? undefined : { packages: names }),
  })
}

// Listing runs an apt-get simulation on the server, so poll it rarely.
const { refreshing } = useAutoRefresh(load, 60_000)
onBeforeUnmount(() => window.clearTimeout(jobTimer))
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <h1 class="page-title">{{ t.aptTitle }}</h1>
        <p class="page-subtitle">{{ t.aptSubtitle }}</p>
      </div>
      <div v-if="isAdmin" class="flex flex-wrap gap-2">
        <Button
          :label="t.aptCheck"
          icon="pi pi-sync"
          severity="secondary"
          outlined
          :loading="starting && !running"
          :disabled="running"
          @click="refresh"
        />
        <Button
          v-if="selected.length"
          :label="t.aptUpgradeSelected.replace('{n}', String(selected.length))"
          icon="pi pi-check-square"
          severity="secondary"
          :disabled="running"
          @click="upgrade(false)"
        />
        <Button
          :label="t.aptUpgradeAll"
          icon="pi pi-download"
          :disabled="running || !packages.length"
          @click="upgrade(true)"
        />
      </div>
    </header>

    <Message v-if="status?.reboot_required" severity="warn" :closable="false" class="mb-4">
      <i class="pi pi-refresh mr-2" />{{ t.rebootRequired }}
      <template v-if="status.reboot_packages.length">
        {{ t.aptRebootPackages }}:
        <span class="font-mono text-sm">{{ status.reboot_packages.join(', ') }}</span>
      </template>
    </Message>

    <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div class="panel-card stat-card">
        <div class="stat-card__label">
          <span class="stat-icon"><i class="pi pi-box" /></span>{{ t.aptAvailable }}
        </div>
        <Skeleton v-if="!status" width="3rem" height="2rem" class="mt-2" />
        <div v-else class="stat-card__value">{{ packages.length }}</div>
      </div>
      <div class="panel-card stat-card">
        <div class="stat-card__label">
          <span class="stat-icon"><i class="pi pi-shield" /></span>{{ t.aptSecurity }}
        </div>
        <Skeleton v-if="!status" width="3rem" height="2rem" class="mt-2" />
        <div
          v-else
          class="stat-card__value"
          :class="status.security_count ? 'text-[var(--app-danger)]' : ''"
        >
          {{ status.security_count }}
        </div>
      </div>
      <div class="panel-card stat-card">
        <div class="stat-card__label">
          <span class="stat-icon"><i class="pi pi-clock" /></span>{{ t.aptLastCheck }}
        </div>
        <Skeleton v-if="!status" width="6rem" height="2rem" class="mt-2" />
        <div
          v-else
          v-tooltip.bottom="
            status.last_refresh ? formatDateTime(status.last_refresh, locale) : undefined
          "
          class="stat-card__value !text-xl"
        >
          {{ status.last_refresh ? formatRelativeTime(status.last_refresh, locale) : t.aptNever }}
        </div>
      </div>
      <div class="panel-card stat-card">
        <div class="stat-card__label">
          <span class="stat-icon"><i class="pi pi-calendar-clock" /></span>{{ t.aptAutoUpdates }}
        </div>
        <Skeleton v-if="!status" width="6rem" height="2rem" class="mt-2" />
        <div v-else class="mt-3">
          <Tag
            :severity="status.auto_upgrades.enabled ? 'success' : 'secondary'"
            :icon="status.auto_upgrades.enabled ? 'pi pi-check' : 'pi pi-minus'"
            :value="autoLabel"
          />
        </div>
      </div>
    </div>

    <div v-if="job.state !== 'idle'" class="panel-card mt-4 overflow-hidden">
      <div class="flex flex-wrap items-center gap-3 border-b border-[var(--app-border)] px-4 py-3">
        <i :class="running ? 'pi pi-spin pi-spinner' : 'pi pi-history'" class="muted" />
        <span class="font-medium">
          {{ running ? '' : t.aptLastJob + ': '
          }}{{ job.operation === 'refresh' ? t.aptOpRefresh : t.aptOpUpgrade }}
        </span>
        <Tag :severity="jobSeverity" :value="jobLabel" />
        <span v-if="job.exit_code !== undefined && job.state === 'failed'" class="muted text-sm">
          {{ t.aptExitCode }}: <span class="font-mono">{{ job.exit_code }}</span>
        </span>
        <span class="flex-1" />
        <span v-if="job.started_at" class="muted text-sm">{{
          formatDateTime(job.started_at, locale)
        }}</span>
      </div>
      <pre ref="logHost" class="apt-log" aria-live="polite">{{
        job.log.length ? job.log.join('\n') : t.aptNoOutput
      }}</pre>
    </div>

    <DataTable
      v-model:selection="selected"
      :value="packages"
      data-key="name"
      :loading="!status || refreshing"
      size="small"
      scrollable
      scroll-height="calc(100vh - 360px)"
      :virtual-scroller-options="packages.length > 60 ? { itemSize: 44 } : undefined"
      class="mt-4"
    >
      <template #empty>
        <div v-if="status" class="grid place-items-center py-12 text-center">
          <span class="stat-icon mb-3 !h-11 !w-11"><i class="pi pi-check text-lg" /></span>
          <strong>{{ t.aptUpToDate }}</strong>
          <p class="muted mb-0 mt-1 max-w-md text-sm">{{ t.aptUpToDateHint }}</p>
        </div>
      </template>
      <Column v-if="isAdmin" selection-mode="multiple" header-style="width: 3rem" />
      <Column field="name" :header="t.aptPackage" sortable>
        <template #body="{ data }">
          <span class="font-mono text-sm">{{ data.name }}</span>
          <Tag
            v-if="data.security"
            severity="danger"
            icon="pi pi-shield"
            :value="t.aptSecurity"
            class="ml-2 !text-[0.7rem]"
          />
          <Tag
            v-if="data.new"
            severity="info"
            :value="t.aptNewPackage"
            class="ml-2 !text-[0.7rem]"
          />
        </template>
      </Column>
      <Column field="current" :header="t.aptCurrent">
        <template #body="{ data }"
          ><span class="muted font-mono text-xs">{{ data.current || '—' }}</span></template
        >
      </Column>
      <Column field="candidate" :header="t.aptCandidate">
        <template #body="{ data }"
          ><span class="font-mono text-xs">{{ data.candidate }}</span></template
        >
      </Column>
      <Column field="origin" :header="t.aptOrigin">
        <template #body="{ data }"
          ><span class="muted text-xs">{{ data.origin }}</span></template
        >
      </Column>
    </DataTable>
  </section>
</template>

<style scoped>
.apt-log {
  max-height: 320px;
  margin: 0;
  overflow: auto;
  padding: 14px 16px;
  font-family: var(--app-font-mono);
  font-size: 0.78rem;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--terminal-foreground);
  background: var(--terminal-background);
}
</style>
