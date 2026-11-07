<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Skeleton from 'primevue/skeleton'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import RefreshControls from '../components/RefreshControls.vue'
import { useAutoRefresh } from '../composables/useAutoRefresh'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'
import { useTableRow } from '../composables/useTableRow'

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

interface DockerImage {
  id: string
  tags: string[]
  size: number
  containers: number
  created_at: string
}

interface DockerVolume {
  name: string
  driver: string
  mountpoint: string
  created_at: string
}

interface DockerNetwork {
  id: string
  name: string
  driver: string
  scope: string
  internal: boolean
}

interface PullJob {
  id: string
  image: string
  status: 'running' | 'done' | 'failed'
  error?: string
  started_at: string
}

type ContainerAction =
  'start' | 'stop' | 'restart' | 'pause' | 'unpause' | 'kill' | 'remove' | 'force-remove'
type PruneKind = 'containers' | 'images' | 'volumes' | 'networks'

const auth = useAuthStore()
const confirm = useConfirm()
const toast = useToast()
const { t, locale } = useI18n()
const { rowSize } = useTableRow()
const isAdmin = computed(() => auth.role === 'admin')
const tab = ref('containers')

interface ResourceLimits {
  cpus: number
  memory_mb: number
}
interface ContainerStats {
  cpu_percent: number
  memory_usage: number
  memory_limit: number
  memory_percent: number
  network_rx: number
  network_tx: number
  block_read: number
  block_write: number
  pids: number
  limits: ResourceLimits
  error: boolean
  sampled_at: string
}
const stats = ref<Record<string, ContainerStats>>({})
const statsFailed = ref(false)
const statsLoading = ref(false)
const limitsTarget = ref<DockerContainer>()
const limitsVisible = ref(false)
const limitsSaving = ref(false)
const limitsForm = reactive<ResourceLimits>({ cpus: 0, memory_mb: 0 })
// Docker samples CPU over about a second per container, so stats arrive after the list:
// the table stays usable and only the metric cells wait.
async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = (await api.get<Record<string, ContainerStats>>('/docker/stats')).data
    statsFailed.value = Object.values(stats.value).some((s) => s.error)
  } catch {
    statsFailed.value = true
  } finally {
    statsLoading.value = false
  }
}
function liveStats(container: DockerContainer) {
  const value = stats.value[container.id]
  return container.state === 'running' && value && !value.error ? value : undefined
}
function shortImage(image: string) {
  return image.startsWith('sha256:') ? image.slice(7, 19) : image
}
function openLimits(container: DockerContainer) {
  limitsTarget.value = container
  Object.assign(limitsForm, stats.value[container.id]?.limits || { cpus: 0, memory_mb: 0 })
  limitsVisible.value = true
}
function confirmLimits() {
  confirm.require({
    header: t.value.resourceLimits,
    message: `${limitsTarget.value?.name}: ${t.value.resourceHint}`,
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.save,
    accept: saveLimits,
  })
}
async function saveLimits() {
  if (!limitsTarget.value) return
  limitsSaving.value = true
  try {
    await api.put(`/docker/containers/${limitsTarget.value.id}/limits`, limitsForm)
    limitsVisible.value = false
    toast.add({ severity: 'success', summary: t.value.resourceSaved, life: 3000 })
    await loadStats()
  } finally {
    limitsSaving.value = false
  }
}
const containers = ref<DockerContainer[]>([])
const images = ref<DockerImage[]>([])
const volumes = ref<DockerVolume[]>([])
const networks = ref<DockerNetwork[]>([])
const pullJobs = ref<PullJob[]>([])
const query = ref('')
const loading = ref(false)
const busyID = ref('')
let pullTimer: number | undefined

const filtered = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return containers.value
  return containers.value.filter((item) =>
    [item.name, item.image, item.state, item.status, item.id].some((field) =>
      field.toLocaleLowerCase().includes(value),
    ),
  )
})

function formatBytes(bytes: number) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}

async function loadContainers(background = false) {
  containers.value = (
    await api.get<DockerContainer[]>('/docker/containers', { silent: background })
  ).data
}
async function loadImages() {
  images.value = (await api.get<DockerImage[]>('/docker/images')).data
}
async function loadVolumes() {
  volumes.value = (await api.get<DockerVolume[]>('/docker/volumes')).data
}
async function loadNetworks() {
  networks.value = (await api.get<DockerNetwork[]>('/docker/networks')).data
}
async function loadPullJobs() {
  pullJobs.value = (await api.get<PullJob[]>('/docker/images/pulls')).data
  const running = pullJobs.value.some((job) => job.status === 'running')
  if (running && !pullTimer) {
    pullTimer = window.setInterval(async () => {
      const wasRunning = pullJobs.value.filter((job) => job.status === 'running').length
      await loadPullJobs()
      const stillRunning = pullJobs.value.filter((job) => job.status === 'running').length
      if (stillRunning < wasRunning) await loadImages()
    }, 2000)
  } else if (!running && pullTimer) {
    window.clearInterval(pullTimer)
    pullTimer = undefined
  }
}

async function load() {
  loading.value = true
  void loadStats()
  try {
    await Promise.allSettled([
      loadContainers(),
      loadImages(),
      loadVolumes(),
      loadNetworks(),
      loadPullJobs(),
    ])
  } finally {
    loading.value = false
  }
}

function stateSeverity(state: string) {
  if (state === 'running') return 'success'
  if (state === 'paused' || state === 'restarting') return 'warn'
  if (state === 'dead' || state === 'exited') return 'danger'
  return 'secondary'
}

function healthSeverity(health: string) {
  if (health === 'healthy') return 'success'
  if (health === 'unhealthy') return 'danger'
  if (health === 'starting') return 'warn'
  return 'secondary'
}

function requestAction(container: DockerContainer, action: ContainerAction) {
  const destructive = ['remove', 'force-remove', 'kill', 'stop'].includes(action)
  confirm.require({
    header: t.value.containerAction,
    message: `${t.value.containerActionConfirm}: ${container.name} — ${t.value[action]}`,
    icon: destructive ? 'pi pi-exclamation-triangle' : 'pi pi-box',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.confirm,
    acceptClass: destructive ? 'p-button-danger' : undefined,
    accept: async () => {
      busyID.value = container.id
      try {
        await api.post(`/docker/containers/${container.id}/action`, { action }, { timeout: 30000 })
        toast.add({ severity: 'success', summary: t.value.containerActionDone, life: 3000 })
        await loadContainers()
      } finally {
        busyID.value = ''
      }
    },
  })
}

// Secondary container actions live in a popup menu to keep rows compact.
const menu = ref<InstanceType<typeof Menu>>()
const menuTarget = ref<DockerContainer>()
const menuItems = computed(() => {
  const container = menuTarget.value
  if (!container) return []
  const running = container.state === 'running'
  const paused = container.state === 'paused'
  const items: Array<{ label: string; icon: string; action: ContainerAction; visible: boolean }> = [
    { label: t.value.restart, icon: 'pi pi-refresh', action: 'restart', visible: running },
    { label: t.value.pause, icon: 'pi pi-pause', action: 'pause', visible: running },
    { label: t.value.unpause, icon: 'pi pi-play', action: 'unpause', visible: paused },
    { label: t.value.kill, icon: 'pi pi-bolt', action: 'kill', visible: running || paused },
    { label: t.value.remove, icon: 'pi pi-trash', action: 'remove', visible: !running && !paused },
    {
      label: t.value['force-remove'],
      icon: 'pi pi-times-circle',
      action: 'force-remove',
      visible: isAdmin.value,
    },
  ]
  return [
    {
      label: t.value.containerLogs,
      icon: 'pi pi-align-left',
      command: () => openLogs(container),
    },
    ...(isAdmin.value
      ? [
          {
            label: t.value.resourceLimits,
            icon: 'pi pi-sliders-h',
            command: () => openLimits(container),
          },
        ]
      : []),
    ...items
      .filter((item) => item.visible)
      .map((item) => ({
        label: item.label,
        icon: item.icon,
        command: () => requestAction(container, item.action),
      })),
  ]
})

function openMenu(event: Event, container: DockerContainer) {
  menuTarget.value = container
  menu.value?.toggle(event)
}

// Logs
const logsVisible = ref(false)
const logsLoading = ref(false)
const logsContainer = ref<DockerContainer>()
const logsText = ref('')
const logsTail = ref(300)
const logsBox = ref<HTMLElement>()

async function loadLogs() {
  if (!logsContainer.value) return
  logsLoading.value = true
  try {
    const response = await api.get<{ logs: string }>(
      `/docker/containers/${logsContainer.value.id}/logs`,
      { params: { tail: logsTail.value } },
    )
    logsText.value = response.data.logs
    requestAnimationFrame(() => {
      if (logsBox.value) logsBox.value.scrollTop = logsBox.value.scrollHeight
    })
  } finally {
    logsLoading.value = false
  }
}

function openLogs(container: DockerContainer) {
  logsContainer.value = container
  logsText.value = ''
  logsVisible.value = true
  loadLogs()
}

// Run container
interface PortRow {
  host_port: number | null
  container_port: number | null
  protocol: 'tcp' | 'udp'
  host_ip: string
}
interface VolumeRow {
  source: string
  target: string
  read_only: boolean
}
const runVisible = ref(false)
const running = ref(false)
const restartPolicies = ['no', 'always', 'unless-stopped', 'on-failure']
const protocols = ['tcp', 'udp']
const runForm = reactive({
  image: '',
  name: '',
  restart_policy: 'unless-stopped',
  env: '',
  start: true,
  cpus: 0 as number | null,
  memory_mb: 0 as number | null,
  ports: [] as PortRow[],
  volumes: [] as VolumeRow[],
})

function openRun(image = '') {
  Object.assign(runForm, {
    image,
    name: '',
    restart_policy: 'unless-stopped',
    env: '',
    start: true,
    cpus: 0 as number | null,
    memory_mb: 0 as number | null,
    ports: [{ host_port: null, container_port: null, protocol: 'tcp', host_ip: '' }],
    volumes: [],
  })
  runVisible.value = true
}

async function runContainer() {
  running.value = true
  try {
    await api.post(
      '/docker/containers',
      {
        image: runForm.image.trim(),
        name: runForm.name.trim(),
        restart_policy: runForm.restart_policy,
        start: runForm.start,
        limits: { cpus: runForm.cpus || 0, memory_mb: runForm.memory_mb || 0 },
        env: runForm.env
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean),
        ports: runForm.ports
          .filter((port) => port.container_port)
          .map((port) => ({
            host_port: port.host_port || 0,
            container_port: port.container_port,
            protocol: port.protocol,
            host_ip: port.host_ip.trim(),
          })),
        volumes: runForm.volumes
          .filter((volume) => volume.source.trim() && volume.target.trim())
          .map((volume) => ({
            source: volume.source.trim(),
            target: volume.target.trim(),
            read_only: volume.read_only,
          })),
      },
      { timeout: 30000 },
    )
    runVisible.value = false
    toast.add({ severity: 'success', summary: t.value.containerCreated, life: 3000 })
    tab.value = 'containers'
    await loadContainers()
  } finally {
    running.value = false
  }
}

// Images
const pullVisible = ref(false)
const pullRef = ref('')
const pulling = ref(false)

async function pullImage() {
  pulling.value = true
  try {
    await api.post('/docker/images/pull', { image: pullRef.value.trim() })
    pullVisible.value = false
    pullRef.value = ''
    toast.add({ severity: 'info', summary: t.value.pullStarted, life: 3000 })
    await loadPullJobs()
  } finally {
    pulling.value = false
  }
}

function pullSeverity(status: string) {
  if (status === 'done') return 'success'
  if (status === 'failed') return 'danger'
  return 'info'
}

function removeImage(image: DockerImage, force: boolean) {
  confirm.require({
    header: t.value.removeImage,
    message: `${t.value.removeImage}: ${image.tags.join(', ') || image.id.slice(7, 19)}${force ? ` (${t.value.forceRemove})` : ''}`,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.delete,
    acceptClass: 'p-button-danger',
    accept: async () => {
      await api.delete(`/docker/images/${encodeURIComponent(image.id)}`, {
        params: { force },
        timeout: 30000,
      })
      toast.add({ severity: 'success', summary: t.value.deleted, life: 3000 })
      await loadImages()
    },
  })
}

// Volumes
const volumeVisible = ref(false)
const volumeName = ref('')

async function createVolume() {
  await api.post('/docker/volumes', { name: volumeName.value.trim() })
  volumeVisible.value = false
  volumeName.value = ''
  await loadVolumes()
}

function removeVolume(volume: DockerVolume) {
  confirm.require({
    header: t.value.delete,
    message: `${t.value.removeVolumeConfirm}: ${volume.name}`,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.delete,
    acceptClass: 'p-button-danger',
    accept: async () => {
      await api.delete(`/docker/volumes/${encodeURIComponent(volume.name)}`)
      toast.add({ severity: 'success', summary: t.value.deleted, life: 3000 })
      await loadVolumes()
    },
  })
}

// Networks
const builtinNetworks = ['bridge', 'host', 'none']

function removeNetwork(network: DockerNetwork) {
  confirm.require({
    header: t.value.delete,
    message: `${t.value.removeNetworkConfirm}: ${network.name}`,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.delete,
    acceptClass: 'p-button-danger',
    accept: async () => {
      await api.delete(`/docker/networks/${network.id}`)
      toast.add({ severity: 'success', summary: t.value.deleted, life: 3000 })
      await loadNetworks()
    },
  })
}

// Cleanup
const pruneOptions = computed(() => [
  { kind: 'containers' as PruneKind, label: t.value.pruneContainers, icon: 'pi pi-box' },
  { kind: 'images' as PruneKind, label: t.value.pruneImages, icon: 'pi pi-images' },
  { kind: 'volumes' as PruneKind, label: t.value.pruneVolumes, icon: 'pi pi-database' },
  { kind: 'networks' as PruneKind, label: t.value.pruneNetworks, icon: 'pi pi-sitemap' },
])
const pruning = ref<PruneKind | ''>('')

function prune(kind: PruneKind, label: string) {
  confirm.require({
    header: label,
    message: t.value.pruneConfirm,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.confirm,
    acceptClass: 'p-button-danger',
    accept: async () => {
      pruning.value = kind
      try {
        const report = (
          await api.post<{ deleted: number; space_reclaimed: number }>(
            '/docker/prune',
            { kind },
            { timeout: 35000 },
          )
        ).data
        toast.add({
          severity: 'success',
          summary: t.value.pruneDone,
          detail: `${report.deleted} ${t.value.removedCount}, ${formatBytes(report.space_reclaimed)} ${t.value.reclaimed}`,
          life: 5000,
        })
        await load()
      } finally {
        pruning.value = ''
      }
    },
  })
}

onMounted(load)
// Live container state is polled only while the containers tab is visible.
const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh(async (background) => {
  if (background && tab.value === 'containers')
    await Promise.all([loadContainers(true), loadStats()])
}, 30000)
onBeforeUnmount(() => window.clearInterval(pullTimer))
</script>

<template>
  <section class="page">
    <header class="page-header">
      <div>
        <h1 class="page-title">{{ t.docker }}</h1>
        <p class="page-subtitle">{{ t.dockerHint }}</p>
      </div>
      <div class="page-actions">
        <Button
          v-if="isAdmin"
          :label="t.pullImage"
          icon="pi pi-download"
          severity="secondary"
          @click="pullVisible = true"
        />
        <Button v-if="isAdmin" :label="t.runContainer" icon="pi pi-play" @click="openRun()" />
        <RefreshControls
          :loading="loading"
          :auto="autoRefresh"
          @refresh="load()"
          @toggle="toggleAutoRefresh"
        />
      </div>
    </header>

    <div v-if="pullJobs.length" class="flex flex-wrap items-center gap-2 text-sm">
      <span class="muted">{{ t.pullJobs }}:</span>
      <Tag
        v-for="job in pullJobs.slice(0, 5)"
        :key="job.id"
        :severity="pullSeverity(job.status)"
        :icon="job.status === 'running' ? 'pi pi-spin pi-spinner' : undefined"
        :value="`${job.image} — ${job.status === 'running' ? t.pullRunning : job.status === 'done' ? t.pullDone : t.pullFailed}`"
        :title="job.error"
      />
    </div>

    <Tabs v-model:value="tab">
      <TabList>
        <Tab value="containers">{{ t.containers }} ({{ containers.length }})</Tab>
        <Tab value="images">{{ t.images }} ({{ images.length }})</Tab>
        <Tab value="volumes">{{ t.volumes }} ({{ volumes.length }})</Tab>
        <Tab value="networks">{{ t.networks }} ({{ networks.length }})</Tab>
        <Tab v-if="isAdmin" value="cleanup">{{ t.cleanup }}</Tab>
      </TabList>
      <TabPanels>
        <TabPanel value="containers" class="space-y-3">
          <div class="flex flex-wrap items-center gap-3">
            <InputText
              v-model="query"
              :placeholder="t.searchContainers"
              class="w-full sm:max-w-sm"
            />
            <span class="muted text-xs">
              <i v-if="statsLoading" class="pi pi-spin pi-spinner mr-1 text-[0.7rem]" />{{
                t.statsHint
              }}
            </span>
          </div>
          <Message v-if="statsFailed" severity="warn" :closable="false">{{
            t.statsFailed
          }}</Message>
          <DataTable
            :value="filtered"
            :loading="loading && !containers.length"
            scrollable
            scroll-height="calc(100vh - 19rem)"
            size="small"
            striped-rows
            data-key="id"
            :virtual-scroller-options="{ itemSize: rowSize }"
            class="vt min-h-80"
          >
            <Column field="name" :header="t.containerName" sortable>
              <template #body="{ data }">
                <span class="cell-clip font-medium [--clip:16rem]" :title="data.name">{{
                  data.name
                }}</span>
              </template>
            </Column>
            <Column field="image" :header="t.image" sortable>
              <template #body="{ data }">
                <span class="cell-clip muted [--clip:14rem]" :title="data.image">{{
                  shortImage(data.image)
                }}</span>
              </template>
            </Column>
            <Column field="state" :header="t.status" sortable>
              <template #body="{ data }">
                <span class="flex items-center gap-1">
                  <Tag
                    :value="data.state"
                    :severity="stateSeverity(data.state)"
                    :title="data.status"
                  />
                  <Tag
                    v-if="data.health && data.health !== 'none'"
                    :value="data.health"
                    :severity="healthSeverity(data.health)"
                    :title="t.health"
                  />
                </span>
              </template>
            </Column>
            <Column :header="t.statsCPU">
              <template #body="{ data }">
                <span v-if="liveStats(data)" class="tabular-nums"
                  >{{ liveStats(data)!.cpu_percent.toFixed(1) }}%</span
                >
                <Skeleton
                  v-else-if="statsLoading && data.state === 'running'"
                  width="3rem"
                  height="0.9rem"
                />
                <span v-else class="muted">—</span>
              </template>
            </Column>
            <Column :header="t.statsMemory">
              <template #body="{ data }">
                <span v-if="liveStats(data)" class="tabular-nums"
                  >{{ formatBytes(liveStats(data)!.memory_usage) }}
                  <span class="muted"
                    >/ {{ formatBytes(liveStats(data)!.memory_limit) }}</span
                  ></span
                >
                <Skeleton
                  v-else-if="statsLoading && data.state === 'running'"
                  width="6rem"
                  height="0.9rem"
                />
                <span v-else class="muted">—</span>
              </template>
            </Column>
            <Column :header="t.statsNetwork">
              <template #body="{ data }">
                <span v-if="liveStats(data)" class="tabular-nums"
                  >{{ formatBytes(liveStats(data)!.network_rx) }} /
                  {{ formatBytes(liveStats(data)!.network_tx) }}</span
                >
                <Skeleton
                  v-else-if="statsLoading && data.state === 'running'"
                  width="6rem"
                  height="0.9rem"
                />
                <span v-else class="muted">—</span>
              </template>
            </Column>
            <Column :header="t.statsDisk">
              <template #body="{ data }">
                <span v-if="liveStats(data)" class="tabular-nums"
                  >{{ formatBytes(liveStats(data)!.block_read) }} /
                  {{ formatBytes(liveStats(data)!.block_write) }}</span
                >
                <Skeleton
                  v-else-if="statsLoading && data.state === 'running'"
                  width="6rem"
                  height="0.9rem"
                />
                <span v-else class="muted">—</span>
              </template>
            </Column>
            <Column :header="t.statsPIDs">
              <template #body="{ data }">
                <span v-if="liveStats(data)" class="tabular-nums">{{ liveStats(data)!.pids }}</span>
                <span v-else class="muted">—</span>
              </template>
            </Column>
            <Column field="ports" :header="t.ports">
              <template #body="{ data }">
                <span
                  class="cell-clip font-mono text-xs [--clip:12rem]"
                  :title="data.ports.join('\n')"
                  >{{ data.ports.join(', ') || '—' }}</span
                >
              </template>
            </Column>
            <Column field="created_at" :header="t.created" sortable>
              <template #body="{ data }">
                <span class="muted">{{ formatDateTime(data.created_at, locale) }}</span>
              </template>
            </Column>
            <Column :header="t.actions" frozen align-frozen="right">
              <template #body="{ data }">
                <div class="flex justify-end gap-1">
                  <Button
                    v-if="data.state !== 'running' && data.state !== 'paused'"
                    v-tooltip.top="t.start"
                    icon="pi pi-play"
                    size="small"
                    text
                    severity="success"
                    :aria-label="t.start"
                    :loading="busyID === data.id"
                    @click="requestAction(data, 'start')"
                  />
                  <Button
                    v-if="data.state === 'running' || data.state === 'paused'"
                    v-tooltip.top="t.stop"
                    icon="pi pi-stop"
                    size="small"
                    text
                    severity="danger"
                    :aria-label="t.stop"
                    :loading="busyID === data.id"
                    @click="requestAction(data, 'stop')"
                  />
                  <Button
                    v-tooltip.top="t.containerLogs"
                    icon="pi pi-align-left"
                    size="small"
                    text
                    :aria-label="t.containerLogs"
                    @click="openLogs(data)"
                  />
                  <Button
                    icon="pi pi-ellipsis-v"
                    size="small"
                    text
                    severity="secondary"
                    :aria-label="t.actions"
                    @click="openMenu($event, data)"
                  />
                </div>
              </template>
            </Column>
            <template #empty>{{ t.noContainers }}</template>
          </DataTable>
          <Menu ref="menu" :model="menuItems" popup />
        </TabPanel>

        <TabPanel value="images">
          <DataTable
            :value="images"
            :loading="loading"
            scrollable
            scroll-height="calc(100vh - 17rem)"
            size="small"
            striped-rows
            data-key="id"
            :virtual-scroller-options="{ itemSize: rowSize }"
            class="vt"
          >
            <Column :header="t.tags">
              <template #body="{ data }">
                <div class="flex items-center gap-1" :title="data.tags.join(', ')">
                  <Tag
                    v-if="data.tags.length"
                    :value="data.tags[0]"
                    severity="secondary"
                    class="max-w-[22rem] truncate"
                  />
                  <span v-if="data.tags.length > 1" class="muted text-xs"
                    >+{{ data.tags.length - 1 }}</span
                  >
                  <span v-if="!data.tags.length" class="muted">&lt;none&gt;</span>
                </div>
              </template>
            </Column>
            <Column field="id" header="ID">
              <template #body="{ data }">
                <span class="font-mono text-xs">{{
                  data.id.replace('sha256:', '').slice(0, 12)
                }}</span>
              </template>
            </Column>
            <Column field="size" :header="t.size" sortable>
              <template #body="{ data }">{{ formatBytes(data.size) }}</template>
            </Column>
            <Column field="created_at" :header="t.created" sortable>
              <template #body="{ data }">{{ formatDateTime(data.created_at, locale) }}</template>
            </Column>
            <Column v-if="isAdmin" :header="t.actions" class="w-36">
              <template #body="{ data }">
                <div class="flex gap-1">
                  <Button
                    v-tooltip.top="t.runContainer"
                    icon="pi pi-play"
                    size="small"
                    text
                    severity="success"
                    :aria-label="t.runContainer"
                    :disabled="!data.tags.length"
                    @click="openRun(data.tags[0])"
                  />
                  <Button
                    v-tooltip.top="t.removeImage"
                    icon="pi pi-trash"
                    size="small"
                    text
                    severity="danger"
                    :aria-label="t.removeImage"
                    @click="removeImage(data, false)"
                  />
                  <Button
                    v-tooltip.top="`${t.removeImage} (${t.forceRemove})`"
                    icon="pi pi-times-circle"
                    size="small"
                    text
                    severity="danger"
                    :aria-label="t.forceRemove"
                    @click="removeImage(data, true)"
                  />
                </div>
              </template>
            </Column>
            <template #empty>{{ t.noImages }}</template>
          </DataTable>
        </TabPanel>

        <TabPanel value="volumes" class="space-y-3">
          <Button
            v-if="isAdmin"
            :label="t.createVolume"
            icon="pi pi-plus"
            size="small"
            @click="volumeVisible = true"
          />
          <DataTable :value="volumes" :loading="loading" size="small" striped-rows data-key="name">
            <Column field="name" :header="t.name" sortable>
              <template #body="{ data }">
                <span class="font-mono text-xs break-all">{{ data.name }}</span>
              </template>
            </Column>
            <Column field="driver" :header="t.driver" />
            <Column field="mountpoint" :header="t.mountpoint">
              <template #body="{ data }">
                <span class="font-mono text-xs break-all">{{ data.mountpoint }}</span>
              </template>
            </Column>
            <Column v-if="isAdmin" :header="t.actions" class="w-20">
              <template #body="{ data }">
                <Button
                  icon="pi pi-trash"
                  size="small"
                  text
                  severity="danger"
                  :aria-label="t.delete"
                  @click="removeVolume(data)"
                />
              </template>
            </Column>
            <template #empty>{{ t.noVolumes }}</template>
          </DataTable>
        </TabPanel>

        <TabPanel value="networks">
          <DataTable :value="networks" :loading="loading" size="small" striped-rows data-key="id">
            <Column field="name" :header="t.name" sortable />
            <Column field="id" header="ID">
              <template #body="{ data }">
                <span class="font-mono text-xs">{{ data.id.slice(0, 12) }}</span>
              </template>
            </Column>
            <Column field="driver" :header="t.driver" sortable />
            <Column field="scope" :header="t.scope" />
            <Column v-if="isAdmin" :header="t.actions" class="w-20">
              <template #body="{ data }">
                <Button
                  v-if="!builtinNetworks.includes(data.name)"
                  icon="pi pi-trash"
                  size="small"
                  text
                  severity="danger"
                  :aria-label="t.delete"
                  @click="removeNetwork(data)"
                />
              </template>
            </Column>
            <template #empty>{{ t.noNetworks }}</template>
          </DataTable>
        </TabPanel>

        <TabPanel v-if="isAdmin" value="cleanup">
          <div class="grid gap-3 sm:grid-cols-2">
            <Button
              v-for="option in pruneOptions"
              :key="option.kind"
              :label="option.label"
              :icon="option.icon"
              severity="danger"
              outlined
              class="!justify-start"
              :loading="pruning === option.kind"
              @click="prune(option.kind, option.label)"
            />
          </div>
        </TabPanel>
      </TabPanels>
    </Tabs>

    <Dialog
      v-model:visible="limitsVisible"
      modal
      :header="`${t.resourceLimits}: ${limitsTarget?.name || ''}`"
      class="w-[min(36rem,96vw)]"
    >
      <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="confirmLimits">
        <p class="muted text-sm sm:col-span-2">{{ t.resourceHint }}</p>
        <div class="flex flex-col gap-2">
          <label for="limits-cpu">{{ t.resourceCPU }}</label
          ><InputNumber
            v-model="limitsForm.cpus"
            input-id="limits-cpu"
            :min="0"
            :max="1024"
            :min-fraction-digits="0"
            :max-fraction-digits="2"
            :step="0.25"
          />
        </div>
        <div class="flex flex-col gap-2">
          <label for="limits-ram">{{ t.resourceMemory }}</label
          ><InputNumber
            v-model="limitsForm.memory_mb"
            input-id="limits-ram"
            :min="0"
            :max="16777216"
            :use-grouping="false"
          />
        </div>
        <div class="flex justify-end gap-2 sm:col-span-2">
          <Button :label="t.cancel" severity="secondary" @click="limitsVisible = false" /><Button
            type="submit"
            :label="t.save"
            :loading="limitsSaving"
          />
        </div>
      </form>
    </Dialog>
    <Dialog
      v-model:visible="logsVisible"
      modal
      maximizable
      :header="`${t.containerLogs}: ${logsContainer?.name || ''}`"
      class="w-[min(64rem,96vw)]"
    >
      <div class="mb-3 flex items-center gap-2">
        <label class="text-sm" for="logs-tail">{{ t.logLines }}</label>
        <Select
          v-model="logsTail"
          input-id="logs-tail"
          :options="[100, 300, 1000, 5000]"
          size="small"
          @change="loadLogs"
        />
        <Button
          icon="pi pi-refresh"
          size="small"
          severity="secondary"
          :aria-label="t.refresh"
          :loading="logsLoading"
          @click="loadLogs"
        />
      </div>
      <pre
        ref="logsBox"
        class="h-[60vh] overflow-auto rounded-md bg-black/90 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-green-100"
        >{{ logsText || (logsLoading ? '…' : t.noLogOutput) }}</pre>
    </Dialog>

    <Dialog v-model:visible="runVisible" modal :header="t.runContainer" class="w-[min(48rem,96vw)]">
      <form class="space-y-4" @submit.prevent="runContainer">
        <p class="muted text-sm">{{ t.runContainerHint }}</p>
        <div class="grid gap-3 sm:grid-cols-2">
          <p class="muted text-sm sm:col-span-2">{{ t.resourceHint }}</p>
          <div class="flex flex-col gap-2">
            <label for="run-cpu">{{ t.resourceCPU }}</label
            ><InputNumber
              v-model="runForm.cpus"
              input-id="run-cpu"
              :min="0"
              :max="1024"
              :min-fraction-digits="0"
              :max-fraction-digits="2"
              :step="0.25"
            />
          </div>
          <div class="flex flex-col gap-2">
            <label for="run-ram">{{ t.resourceMemory }}</label
            ><InputNumber
              v-model="runForm.memory_mb"
              input-id="run-ram"
              :min="0"
              :max="16777216"
              :use-grouping="false"
            />
          </div>
          <InputText v-model="runForm.image" :placeholder="t.imageRef" required autofocus />
          <InputText v-model="runForm.name" :placeholder="t.containerNameOptional" />
          <div class="flex flex-col gap-1">
            <label class="text-sm" for="restart-policy">{{ t.restartPolicy }}</label>
            <Select
              v-model="runForm.restart_policy"
              input-id="restart-policy"
              :options="restartPolicies"
            />
          </div>
          <label class="flex items-center gap-2 self-end pb-2 text-sm">
            <Checkbox v-model="runForm.start" binary />
            {{ t.startAfterCreate }}
          </label>
        </div>

        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-medium">{{ t.portMappings }}</span>
            <Button
              icon="pi pi-plus"
              size="small"
              text
              :aria-label="t.add"
              @click="
                runForm.ports.push({
                  host_port: null,
                  container_port: null,
                  protocol: 'tcp',
                  host_ip: '',
                })
              "
            />
          </div>
          <div
            v-for="(port, index) in runForm.ports"
            :key="`port-${index}`"
            class="grid grid-cols-2 gap-2 sm:grid-cols-[1fr_1fr_6rem_1fr_auto]"
          >
            <InputNumber
              v-model="port.host_port"
              class="min-w-0"
              input-class="w-full min-w-0"
              :placeholder="t.hostPort"
              :min="1"
              :max="65535"
              :use-grouping="false"
            />
            <InputNumber
              v-model="port.container_port"
              class="min-w-0"
              input-class="w-full min-w-0"
              :placeholder="t.containerPort"
              :min="1"
              :max="65535"
              :use-grouping="false"
            />
            <Select v-model="port.protocol" :options="protocols" class="min-w-0" />
            <InputText v-model="port.host_ip" :placeholder="t.hostIP" class="min-w-0" />
            <Button
              icon="pi pi-times"
              text
              severity="secondary"
              :aria-label="t.delete"
              @click="runForm.ports.splice(index, 1)"
            />
          </div>
        </div>

        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-medium">{{ t.volumeMappings }}</span>
            <Button
              icon="pi pi-plus"
              size="small"
              text
              :aria-label="t.add"
              @click="runForm.volumes.push({ source: '', target: '', read_only: false })"
            />
          </div>
          <div
            v-for="(volume, index) in runForm.volumes"
            :key="`volume-${index}`"
            class="grid grid-cols-2 items-center gap-2 sm:grid-cols-[1fr_1fr_auto_auto]"
          >
            <InputText v-model="volume.source" :placeholder="t.volumeSource" class="min-w-0" />
            <InputText v-model="volume.target" :placeholder="t.volumeTarget" class="min-w-0" />
            <label class="flex items-center gap-2 text-sm">
              <Checkbox v-model="volume.read_only" binary />
              {{ t.readOnly }}
            </label>
            <Button
              icon="pi pi-times"
              text
              severity="secondary"
              :aria-label="t.delete"
              @click="runForm.volumes.splice(index, 1)"
            />
          </div>
        </div>

        <div class="flex flex-col gap-1">
          <label class="font-medium" for="run-env">{{ t.envVariables }}</label>
          <Textarea
            id="run-env"
            v-model="runForm.env"
            rows="4"
            class="font-mono text-sm"
            :placeholder="t.envHint"
          />
        </div>

        <div class="flex justify-end gap-2">
          <Button :label="t.cancel" severity="secondary" text @click="runVisible = false" />
          <Button
            type="submit"
            :label="t.runContainer"
            icon="pi pi-play"
            :loading="running"
            :disabled="!runForm.image.trim()"
          />
        </div>
      </form>
    </Dialog>

    <Dialog v-model:visible="pullVisible" modal :header="t.pullImage" class="w-[min(28rem,96vw)]">
      <form class="space-y-4" @submit.prevent="pullImage">
        <InputText v-model="pullRef" :placeholder="t.imageRef" class="w-full" required autofocus />
        <div class="flex justify-end gap-2">
          <Button :label="t.cancel" severity="secondary" text @click="pullVisible = false" />
          <Button
            type="submit"
            :label="t.pullImage"
            icon="pi pi-download"
            :loading="pulling"
            :disabled="!pullRef.trim()"
          />
        </div>
      </form>
    </Dialog>

    <Dialog
      v-model:visible="volumeVisible"
      modal
      :header="t.createVolume"
      class="w-[min(28rem,96vw)]"
    >
      <form class="space-y-4" @submit.prevent="createVolume">
        <InputText
          v-model="volumeName"
          :placeholder="t.volumeName"
          class="w-full"
          required
          autofocus
        />
        <div class="flex justify-end gap-2">
          <Button :label="t.cancel" severity="secondary" text @click="volumeVisible = false" />
          <Button
            type="submit"
            :label="t.create"
            icon="pi pi-plus"
            :disabled="!volumeName.trim()"
          />
        </div>
      </form>
    </Dialog>
  </section>
</template>
