<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import { useToast } from 'primevue/usetoast'
import TelegramSettings from '../components/TelegramSettings.vue'
import NotificationSettings from '../components/NotificationSettings.vue'
import api from '../services/api'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'
import { usePreferencesStore } from '../stores/preferences'

interface AllowedDirectory {
  id: number
  name: string
  path: string
}

interface Overview {
  hostname: string
  version: string
  go_version: string
  os: string
  architecture: string
  data_dir: string
  log_dir: string
  database_size_bytes: number
  metric_samples: number
  audit_events: number
  active_sessions: number
  allowed_directories: AllowedDirectory[]
}

interface UpdateStatus {
  current: string
  latest: string
  available: boolean
  url: string
}

interface OverviewItem {
  label: string
  value: string
  path?: boolean
}

const preferences = usePreferencesStore()
const auth = useAuthStore()
const toast = useToast()
const { t, locale } = useI18n()
const requestedTab = useRoute().query.tab
const overview = ref<Overview>()
const currentVersion = ref('dev')
const updateStatus = ref<UpdateStatus>()
const checkingUpdates = ref(false)

const presets = [
  { label: 'Aura', value: 'aura' },
  { label: 'Lara', value: 'lara' },
]
const modes = computed(() => [
  { label: t.value.dark, value: 'dark' },
  { label: t.value.light, value: 'light' },
])
const locales = [
  { label: 'Русский', value: 'ru' },
  { label: 'English', value: 'en' },
]
const colors = [
  { label: 'Emerald', value: 'emerald' },
  { label: 'Blue', value: 'blue' },
  { label: 'Violet', value: 'violet' },
]

// Only tabs with their own settings: pages such as Docker, Systemd or Firewall
// are reached from the side menu, not through a tab holding a single link.
const sections = computed(() => {
  const items = [
    { value: 'general', label: t.value.general, icon: 'pi pi-home', roles: [] },
    { value: 'interface', label: t.value.appearance, icon: 'pi pi-palette', roles: [] },
    {
      value: 'directories',
      label: t.value.allowedDirectories,
      icon: 'pi pi-folder-open',
      roles: ['admin', 'operator'],
    },
    { value: 'telegram', label: 'Telegram', icon: 'pi pi-send', roles: ['admin'] },
    { value: 'notifications', label: t.value.notifications, icon: 'pi pi-bell', roles: ['admin'] },
    { value: 'updates', label: t.value.updates, icon: 'pi pi-sync', roles: ['admin'] },
  ]
  return items.filter((item) => !item.roles.length || item.roles.includes(auth.role))
})

const activeSection = ref(
  typeof requestedTab === 'string' && sections.value.some((item) => item.value === requestedTab)
    ? requestedTab
    : 'general',
)

const overviewItems = computed(() => {
  const items: OverviewItem[] = [
    { label: t.value.server, value: overview.value?.hostname || 'Mini Ubuntu Server' },
    { label: t.value.currentVersion, value: currentVersion.value },
    { label: t.value.account, value: `${auth.username} · ${auth.role}` },
  ]
  const data = overview.value
  if (!data) return items
  items.push(
    { label: t.value.activeSessions, value: String(data.active_sessions) },
    { label: 'SQLite', value: formatBytes(data.database_size_bytes) },
    { label: t.value.auditEvents, value: data.audit_events.toLocaleString(locale.value) },
    { label: t.value.storedMetricSamples, value: data.metric_samples.toLocaleString(locale.value) },
    { label: t.value.dataDirectory, value: data.data_dir, path: true },
    { label: t.value.logDirectory, value: data.log_dir, path: true },
  )
  if (auth.role === 'admin') {
    items.push({ label: t.value.backups, value: `${data.data_dir}/backups`, path: true })
  }
  return items
})

function formatBytes(value = 0) {
  if (value < 1024) return `${value} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let amount = value / 1024
  let index = 0
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024
    index++
  }
  return `${new Intl.NumberFormat(locale.value, { maximumFractionDigits: 1 }).format(amount)} ${units[index]}`
}

async function loadOverview() {
  const health = await api.get<{ version: string }>('/health')
  currentVersion.value = health.data.version
  if (auth.role === 'admin' || auth.role === 'operator') {
    const response = await api.get<Overview>('/settings/overview')
    overview.value = response.data
  }
}

async function checkUpdates() {
  checkingUpdates.value = true
  try {
    const response = await api.get<UpdateStatus>('/updates')
    updateStatus.value = response.data
    toast.add({
      severity: response.data.available ? 'info' : 'success',
      summary: response.data.available ? t.value.updateAvailable : t.value.alreadyUpToDate,
      life: 4000,
    })
  } catch {
    // The shared Axios interceptor already reports a localized PrimeVue Toast.
  } finally {
    checkingUpdates.value = false
  }
}

function openRelease() {
  const url = updateStatus.value?.url || ''
  if (url.startsWith('https://github.com/kantaevsherhan/mini-ubuntu-server-panel/releases/')) {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}

onMounted(() => {
  void loadOverview().catch(() => undefined)
})
</script>

<template>
  <section>
    <h1 class="mb-5 text-2xl font-semibold">{{ t.settings }}</h1>
    <Tabs v-model:value="activeSection" scrollable>
      <TabList>
        <Tab v-for="section in sections" :key="section.value" :value="section.value">
          <i :class="section.icon" class="mr-2" />{{ section.label }}
        </Tab>
      </TabList>

      <TabPanels>
        <TabPanel value="general">
          <Card class="max-w-5xl">
            <template #title>{{ t.general }}</template>
            <template #content>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div v-for="item in overviewItems" :key="item.label" class="panel-card px-4 py-3">
                  <span class="muted text-sm">{{ item.label }}</span>
                  <code v-if="item.path" class="mt-1 block break-all">{{ item.value }}</code>
                  <strong v-else class="mt-1 block">{{ item.value }}</strong>
                </div>
              </div>
              <Message severity="info" :closable="false" class="mt-4">{{
                t.securitySessionHint
              }}</Message>
            </template>
          </Card>
        </TabPanel>

        <TabPanel value="interface">
          <Card class="max-w-3xl">
            <template #title>{{ t.appearance }}</template>
            <template #content>
              <div class="grid grid-cols-1 gap-5 md:grid-cols-2">
                <label
                  ><span class="muted mb-2 block text-sm">PrimeVue preset</span
                  ><Select
                    v-model="preferences.preset"
                    :options="presets"
                    option-label="label"
                    option-value="value"
                    class="w-full"
                /></label>
                <label
                  ><span class="muted mb-2 block text-sm">{{ t.colorMode }}</span
                  ><Select
                    v-model="preferences.mode"
                    :options="modes"
                    option-label="label"
                    option-value="value"
                    class="w-full"
                /></label>
                <label
                  ><span class="muted mb-2 block text-sm">{{ t.accentColor }}</span
                  ><Select
                    v-model="preferences.accent"
                    :options="colors"
                    option-label="label"
                    option-value="value"
                    class="w-full"
                /></label>
                <label
                  ><span class="muted mb-2 block text-sm">{{ t.language }}</span
                  ><Select
                    v-model="preferences.locale"
                    :options="locales"
                    option-label="label"
                    option-value="value"
                    class="w-full"
                /></label>
              </div>
            </template>
          </Card>
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin' || auth.role === 'operator'" value="directories">
          <Card
            ><template #title>{{ t.allowedDirectories }}</template
            ><template #content
              ><Message severity="warn" :closable="false" class="mb-4">{{
                t.allowedDirectoriesHint
              }}</Message
              ><DataTable :value="overview?.allowed_directories || []" size="small"
                ><Column field="id" header="#" class="w-20" /><Column
                  field="name"
                  :header="t.name"
                /><Column field="path" :header="t.path"
                  ><template #body="{ data }"
                    ><code>{{ data.path }}</code></template
                  ></Column
                ></DataTable
              ></template
            ></Card
          >
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin'" value="telegram"><TelegramSettings /></TabPanel>
        <TabPanel v-if="auth.role === 'admin'" value="notifications"
          ><NotificationSettings
        /></TabPanel>

        <TabPanel v-if="auth.role === 'admin'" value="updates">
          <Card class="max-w-4xl"
            ><template #title>{{ t.updates }}</template
            ><template #content
              ><div class="grid gap-3 sm:grid-cols-2">
                <div class="panel-card px-4 py-3">
                  <span class="muted text-sm">{{ t.currentVersion }}</span
                  ><strong class="mt-1 block">{{ updateStatus?.current || currentVersion }}</strong>
                </div>
                <div class="panel-card px-4 py-3">
                  <span class="muted text-sm">{{ t.latestVersion }}</span
                  ><strong class="mt-1 block">{{ updateStatus?.latest || '—' }}</strong>
                </div>
              </div>
              <Message
                v-if="updateStatus"
                :severity="updateStatus.available ? 'info' : 'success'"
                :closable="false"
                class="my-4"
                >{{ updateStatus.available ? t.updateAvailable : t.alreadyUpToDate }}</Message
              >
              <div class="mt-4 flex flex-wrap gap-2">
                <Button
                  :label="t.checkUpdates"
                  icon="pi pi-refresh"
                  :loading="checkingUpdates"
                  @click="checkUpdates"
                /><Button
                  v-if="updateStatus?.available"
                  :label="t.openRelease"
                  icon="pi pi-external-link"
                  outlined
                  @click="openRelease"
                /></div></template
          ></Card>
        </TabPanel>
      </TabPanels>
    </Tabs>
  </section>
</template>
