<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
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
import Tag from 'primevue/tag'
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

const preferences = usePreferencesStore()
const auth = useAuthStore()
const router = useRouter()
const toast = useToast()
const { t, locale } = useI18n()
const activeSection = ref('general')
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

const sections = computed(() => {
  const items = [
    { value: 'general', label: t.value.general, icon: 'pi pi-home', roles: [] },
    { value: 'interface', label: t.value.appearance, icon: 'pi pi-palette', roles: [] },
    { value: 'metrics', label: t.value.metrics, icon: 'pi pi-chart-line', roles: [] },
    {
      value: 'storage',
      label: t.value.dataStorage,
      icon: 'pi pi-database',
      roles: ['admin', 'operator'],
    },
    {
      value: 'users',
      label: t.value.usersAndRoles,
      icon: 'pi pi-users',
      roles: ['admin', 'operator'],
    },
    {
      value: 'directories',
      label: t.value.allowedDirectories,
      icon: 'pi pi-folder-open',
      roles: ['admin', 'operator'],
    },
    { value: 'docker', label: 'Docker', icon: 'pi pi-box', roles: ['admin', 'operator'] },
    { value: 'systemd', label: 'Systemd', icon: 'pi pi-cog', roles: ['admin', 'operator'] },
    {
      value: 'firewall',
      label: t.value.firewall,
      icon: 'pi pi-shield',
      roles: ['admin', 'operator'],
    },
    { value: 'telegram', label: 'Telegram', icon: 'pi pi-send', roles: ['admin'] },
    { value: 'notifications', label: t.value.notifications, icon: 'pi pi-bell', roles: ['admin'] },
    { value: 'security', label: t.value.security, icon: 'pi pi-lock', roles: [] },
    { value: 'backups', label: t.value.backups, icon: 'pi pi-save', roles: ['admin'] },
    { value: 'updates', label: t.value.updates, icon: 'pi pi-sync', roles: ['admin'] },
  ]
  return items.filter((item) => !item.roles.length || item.roles.includes(auth.role))
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
          <Card class="max-w-4xl">
            <template #title>{{ t.general }}</template>
            <template #content>
              <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.server }}</span>
                  <strong class="mt-1 block">{{
                    overview?.hostname || 'Mini Ubuntu Server'
                  }}</strong>
                </div>
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.currentVersion }}</span>
                  <strong class="mt-1 block">{{ currentVersion }}</strong>
                </div>
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.account }}</span>
                  <strong class="mt-1 block">{{ auth.username }} · {{ auth.role }}</strong>
                </div>
              </div>
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

        <TabPanel value="metrics">
          <Card class="max-w-4xl">
            <template #title>{{ t.metrics }}</template>
            <template #content>
              <div class="mb-4 flex flex-wrap items-center gap-3">
                <Tag severity="success" :value="t.enabled" /><span>{{
                  t.metricsCollectorHint
                }}</span>
              </div>
              <p v-if="overview" class="muted">
                {{ t.storedMetricSamples }}: {{ overview.metric_samples.toLocaleString(locale) }}
              </p>
              <Button
                :label="t.openDashboard"
                icon="pi pi-chart-line"
                outlined
                @click="router.push('/')"
              />
            </template>
          </Card>
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin' || auth.role === 'operator'" value="storage">
          <Card class="max-w-4xl">
            <template #title>{{ t.dataStorage }}</template>
            <template #content>
              <div class="grid gap-4 md:grid-cols-2">
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.dataDirectory }}</span
                  ><code class="mt-2 block break-all">{{ overview?.data_dir || '—' }}</code>
                </div>
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.logDirectory }}</span
                  ><code class="mt-2 block break-all">{{ overview?.log_dir || '—' }}</code>
                </div>
                <div class="panel-card p-4">
                  <span class="muted text-sm">SQLite</span
                  ><strong class="mt-2 block">{{
                    formatBytes(overview?.database_size_bytes)
                  }}</strong>
                </div>
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.auditEvents }}</span
                  ><strong class="mt-2 block">{{
                    overview?.audit_events.toLocaleString(locale) || '0'
                  }}</strong>
                </div>
              </div>
            </template>
          </Card>
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin' || auth.role === 'operator'" value="users">
          <Card class="max-w-4xl"
            ><template #title>{{ t.usersAndRoles }}</template
            ><template #content
              ><div class="mb-4 flex flex-wrap gap-2">
                <Tag value="admin" severity="danger" /><Tag value="operator" severity="warn" /><Tag
                  value="viewer"
                  severity="secondary"
                />
              </div>
              <p class="muted">{{ t.rolesHint }}</p>
              <Button
                :label="t.openUsers"
                icon="pi pi-users"
                @click="router.push('/users')" /></template
          ></Card>
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

        <TabPanel v-if="auth.role === 'admin' || auth.role === 'operator'" value="docker">
          <Card class="max-w-4xl"
            ><template #title>Docker</template
            ><template #content
              ><Message severity="warn" :closable="false" class="mb-4">{{
                t.dockerPrivilegeHint
              }}</Message
              ><Button
                :label="t.openDocker"
                icon="pi pi-box"
                @click="router.push('/docker')" /></template
          ></Card>
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin' || auth.role === 'operator'" value="systemd">
          <Card class="max-w-4xl"
            ><template #title>Systemd</template
            ><template #content
              ><p class="muted">{{ t.systemdHint }}</p>
              <Button
                :label="t.openServices"
                icon="pi pi-cog"
                @click="router.push('/services')" /></template
          ></Card>
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin' || auth.role === 'operator'" value="firewall">
          <Card class="max-w-4xl"
            ><template #title>{{ t.firewall }}</template
            ><template #content
              ><p class="muted">{{ t.settingsFirewallHint }}</p>
              <Button
                :label="t.openFirewall"
                icon="pi pi-shield"
                @click="router.push('/firewall')" /></template
          ></Card>
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin'" value="telegram"><TelegramSettings /></TabPanel>
        <TabPanel v-if="auth.role === 'admin'" value="notifications"
          ><NotificationSettings
        /></TabPanel>

        <TabPanel value="security">
          <Card class="max-w-4xl"
            ><template #title>{{ t.security }}</template
            ><template #content
              ><Message severity="info" :closable="false" class="mb-4">{{
                t.securitySessionHint
              }}</Message>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.activeSessions }}</span
                  ><strong class="mt-2 block">{{ overview?.active_sessions ?? 1 }}</strong>
                </div>
                <div class="panel-card p-4">
                  <span class="muted text-sm">RBAC</span
                  ><strong class="mt-2 block">{{ auth.role }}</strong>
                </div>
              </div></template
            ></Card
          >
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin'" value="backups">
          <Card class="max-w-4xl"
            ><template #title>{{ t.backups }}</template
            ><template #content
              ><Message severity="info" :closable="false" class="mb-4">{{ t.backupHint }}</Message
              ><code class="panel-card block overflow-auto p-4"
                >/var/lib/mini-ubuntu-server/backups</code
              ></template
            ></Card
          >
        </TabPanel>

        <TabPanel v-if="auth.role === 'admin'" value="updates">
          <Card class="max-w-4xl"
            ><template #title>{{ t.updates }}</template
            ><template #content
              ><div class="grid gap-4 sm:grid-cols-2">
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.currentVersion }}</span
                  ><strong class="mt-2 block">{{ updateStatus?.current || currentVersion }}</strong>
                </div>
                <div class="panel-card p-4">
                  <span class="muted text-sm">{{ t.latestVersion }}</span
                  ><strong class="mt-2 block">{{ updateStatus?.latest || '—' }}</strong>
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
