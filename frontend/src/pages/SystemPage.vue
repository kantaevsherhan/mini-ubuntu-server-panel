<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import RefreshControls from '../components/RefreshControls.vue'
import { useAutoRefresh } from '../composables/useAutoRefresh'
import api from '../services/api'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

interface Disk {
  device: string
  mountpoint: string
  fs_type: string
  total_bytes: number
  used_bytes: number
  free_bytes: number
}

interface NetworkInterface {
  name: string
  mac: string
  up: boolean
  mtu: number
  addresses: string[]
  rx_bytes: number
  tx_bytes: number
}

interface SystemInfo {
  hostname: string
  os: string
  kernel: string
  arch: string
  cpu_count: number
  cpu_model: string
  uptime_seconds: number
  load: [number, number, number]
  memory: {
    total_bytes: number
    available_bytes: number
    swap_total_bytes: number
    swap_free_bytes: number
  }
  disks: Disk[]
  interfaces: NetworkInterface[]
}

interface ListeningPort {
  protocol: 'tcp' | 'udp'
  address: string
  port: number
  pid?: number
  process?: string
  public: boolean
}

const info = ref<SystemInfo>()
const ports = ref<ListeningPort[]>([])
const loading = ref(false)
const query = ref('')
const router = useRouter()
const auth = useAuthStore()
const { t, locale } = useI18n()

const filteredPorts = computed(() => {
  const value = query.value.trim().toLocaleLowerCase()
  if (!value) return ports.value
  return ports.value.filter((port) =>
    [String(port.port), port.address, port.protocol, port.process || '', String(port.pid || '')]
      .join(' ')
      .toLocaleLowerCase()
      .includes(value),
  )
})

function formatBytes(bytes = 0) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}

function percent(used: number, total: number) {
  return total ? Math.round((used / total) * 100) : 0
}

function formatUptime(seconds = 0) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const suffix = locale.value === 'ru' ? ['д', 'ч', 'м'] : ['d', 'h', 'm']
  return `${days}${suffix[0]} ${hours}${suffix[1]} ${minutes}${suffix[2]}`
}

const memoryUsed = computed(() =>
  info.value ? info.value.memory.total_bytes - info.value.memory.available_bytes : 0,
)
const swapUsed = computed(() =>
  info.value ? info.value.memory.swap_total_bytes - info.value.memory.swap_free_bytes : 0,
)

function usageSeverityClass(value: number) {
  if (value >= 90) return 'usage-danger'
  if (value >= 75) return 'usage-warn'
  return ''
}

async function load(background = false) {
  if (!background) loading.value = true
  try {
    const [infoResponse, portsResponse] = await Promise.all([
      api.get<SystemInfo>('/system/info', { silent: background }),
      api.get<ListeningPort[]>('/system/ports', { silent: background }),
    ])
    info.value = infoResponse.data
    ports.value = portsResponse.data
  } finally {
    loading.value = false
  }
}

function openInFirewall(port: ListeningPort) {
  router.push({ path: '/firewall', query: { port: String(port.port), protocol: port.protocol } })
}

const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh(load, 10000)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold">{{ t.system }}</h1>
        <p class="muted mt-1 text-sm">{{ t.systemHint }}</p>
      </div>
      <RefreshControls
        :loading="loading"
        :auto="autoRefresh"
        @refresh="load()"
        @toggle="toggleAutoRefresh"
      />
    </div>

    <div v-if="info" class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <div class="panel-card space-y-1 p-4">
        <div class="muted text-xs uppercase">{{ t.hostname }}</div>
        <div class="text-lg font-semibold break-all">{{ info.hostname }}</div>
        <div class="text-sm">{{ info.os }}</div>
        <div class="muted font-mono text-xs">{{ info.kernel }} · {{ info.arch }}</div>
      </div>
      <div class="panel-card space-y-1 p-4">
        <div class="muted text-xs uppercase">{{ t.cpu }}</div>
        <div class="text-lg font-semibold">{{ info.cpu_count }} {{ t.cores }}</div>
        <div class="truncate text-sm" :title="info.cpu_model">{{ info.cpu_model || '—' }}</div>
        <div class="muted text-xs">
          {{ t.loadAverage }}: <span class="font-mono">{{ info.load.join(' / ') }}</span>
        </div>
      </div>
      <div class="panel-card space-y-2 p-4">
        <div class="muted text-xs uppercase">{{ t.memory }}</div>
        <div class="text-sm">
          {{ formatBytes(memoryUsed) }} / {{ formatBytes(info.memory.total_bytes) }}
        </div>
        <ProgressBar
          :value="percent(memoryUsed, info.memory.total_bytes)"
          :class="usageSeverityClass(percent(memoryUsed, info.memory.total_bytes))"
          class="h-2"
          :show-value="false"
        />
        <div v-if="info.memory.swap_total_bytes" class="muted text-xs">
          {{ t.swap }}: {{ formatBytes(swapUsed) }} /
          {{ formatBytes(info.memory.swap_total_bytes) }}
        </div>
      </div>
      <div class="panel-card space-y-1 p-4">
        <div class="muted text-xs uppercase">{{ t.uptime }}</div>
        <div class="text-lg font-semibold">{{ formatUptime(info.uptime_seconds) }}</div>
      </div>
    </div>

    <div class="space-y-2">
      <h2 class="text-lg font-semibold">{{ t.disks }}</h2>
      <DataTable :value="info?.disks || []" :loading="loading" size="small" data-key="device">
        <Column field="mountpoint" :header="t.mountpoint" sortable>
          <template #body="{ data }"
            ><span class="font-mono">{{ data.mountpoint }}</span></template
          >
        </Column>
        <Column field="device" :header="t.device">
          <template #body="{ data }">
            <span class="font-mono text-xs">{{ data.device }}</span>
          </template>
        </Column>
        <Column field="fs_type" :header="t.fsType" class="w-20" />
        <Column :header="t.used" class="min-w-56">
          <template #body="{ data }">
            <div class="space-y-1">
              <ProgressBar
                :value="percent(data.used_bytes, data.total_bytes)"
                :class="usageSeverityClass(percent(data.used_bytes, data.total_bytes))"
                class="h-2"
                :show-value="false"
              />
              <div class="muted text-xs">
                {{ formatBytes(data.used_bytes) }} / {{ formatBytes(data.total_bytes) }} ·
                {{ percent(data.used_bytes, data.total_bytes) }}%
              </div>
            </div>
          </template>
        </Column>
        <Column field="free_bytes" :header="t.free" sortable>
          <template #body="{ data }">{{ formatBytes(data.free_bytes) }}</template>
        </Column>
      </DataTable>
    </div>

    <div class="space-y-2">
      <h2 class="text-lg font-semibold">{{ t.listeningPorts }}</h2>
      <p class="muted text-sm">{{ t.listeningPortsHint }}</p>
      <InputText v-model="query" :placeholder="t.searchPorts" class="w-full sm:max-w-md" />
      <DataTable
        :value="filteredPorts"
        :loading="loading"
        size="small"
        striped-rows
        scrollable
        scroll-height="28rem"
      >
        <Column field="port" :header="t.port" sortable class="w-24">
          <template #body="{ data }"
            ><span class="font-mono">{{ data.port }}</span></template
          >
        </Column>
        <Column field="protocol" :header="t.protocol" sortable class="w-24" />
        <Column field="address" :header="t.address" sortable>
          <template #body="{ data }">
            <span class="font-mono text-xs">{{ data.address }}</span>
          </template>
        </Column>
        <Column field="public" :header="t.status" sortable class="w-28">
          <template #body="{ data }">
            <Tag
              :value="data.public ? t.publicExposure : t.localOnly"
              :severity="data.public ? 'warn' : 'secondary'"
            />
          </template>
        </Column>
        <Column field="process" :header="t.process" sortable>
          <template #body="{ data }">
            <span v-if="data.process"
              >{{ data.process }} <span class="muted">({{ data.pid }})</span></span
            >
            <span v-else class="muted">—</span>
          </template>
        </Column>
        <Column v-if="auth.role === 'admin'" :header="t.actions" class="w-44">
          <template #body="{ data }">
            <Button
              v-if="data.public"
              :label="t.openInFirewall"
              icon="pi pi-shield"
              size="small"
              text
              @click="openInFirewall(data)"
            />
          </template>
        </Column>
        <template #empty>{{ t.noPorts }}</template>
      </DataTable>
    </div>

    <div class="space-y-2">
      <h2 class="text-lg font-semibold">{{ t.interfaces }}</h2>
      <DataTable :value="info?.interfaces || []" :loading="loading" size="small" data-key="name">
        <Column field="name" :header="t.interfaceName" sortable>
          <template #body="{ data }">
            <div class="flex items-center gap-2">
              <span class="font-mono">{{ data.name }}</span>
              <Tag :value="data.up ? 'UP' : 'DOWN'" :severity="data.up ? 'success' : 'secondary'" />
            </div>
          </template>
        </Column>
        <Column :header="t.addresses">
          <template #body="{ data }">
            <div class="font-mono text-xs">
              <div v-for="address in data.addresses" :key="address">{{ address }}</div>
              <span v-if="!data.addresses.length" class="muted">—</span>
            </div>
          </template>
        </Column>
        <Column field="mac" header="MAC">
          <template #body="{ data }">
            <span class="font-mono text-xs">{{ data.mac || '—' }}</span>
          </template>
        </Column>
        <Column field="mtu" header="MTU" class="w-20" />
        <Column field="rx_bytes" :header="t.received" sortable>
          <template #body="{ data }">{{ formatBytes(data.rx_bytes) }}</template>
        </Column>
        <Column field="tx_bytes" :header="t.sent" sortable>
          <template #body="{ data }">{{ formatBytes(data.tx_bytes) }}</template>
        </Column>
      </DataTable>
    </div>
  </section>
</template>

<style scoped>
.usage-warn :deep(.p-progressbar-value) {
  background: var(--p-orange-500);
}
.usage-danger :deep(.p-progressbar-value) {
  background: var(--p-red-500);
}
</style>
