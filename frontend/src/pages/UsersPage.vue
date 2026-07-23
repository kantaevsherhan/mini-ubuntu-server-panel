<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

interface PanelUser {
  id: number
  username: string
  display_name: string
  role: 'admin' | 'operator' | 'viewer'
  is_active: boolean
  system_username: string | null
  created_at: string
  last_login_at: string | null
}

interface WebSession {
  id: string
  ip_address: string
  user_agent: string
  created_at: string
  last_seen_at: string
  expires_at: string
  revoked_at: string | null
}

interface SystemDetails {
  username: string
  uid: number
  gid: number
  home: string
  shell: string
  groups: string[]
  has_sudo: boolean
  has_ssh_keys: boolean
  last_login_at: string | null
  active_sessions: Array<{ terminal: string; remote_ip: string; started_at: string }>
}

const users = ref<PanelUser[]>([])
const sessions = ref<WebSession[]>([])
const systemDetails = ref<SystemDetails>()
const loading = ref(true)
const saving = ref(false)
const editorVisible = ref(false)
const resetVisible = ref(false)
const sessionsVisible = ref(false)
const deleteVisible = ref(false)
const selected = ref<PanelUser>()
const mode = ref<'create' | 'edit'>('create')
const resetPassword = ref('')
const form = reactive({
  username: '',
  display_name: '',
  password: '',
  role: 'viewer' as PanelUser['role'],
  is_active: true,
  system_username: '',
  create_panel_user: true,
  create_system_user: false,
  home_directory: '',
  shell: '/bin/bash',
  system_groups: [] as string[],
  allow_sudo: false,
  create_home: true,
  allow_ssh: false,
  ssh_public_key: '',
})
const roles = ['admin', 'operator', 'viewer']
const availableGroups = ref<string[]>([])
const deleteOptions = reactive({
  delete_panel_user: true,
  delete_system_user: false,
  delete_home_directory: false,
  delete_ssh_keys: false,
  terminate_sessions: false,
})
const toast = useToast()
const authStore = useAuthStore()
const { t, locale } = useI18n()

async function load() {
  loading.value = true
  try {
    users.value = (await api.get('/users')).data
  } finally {
    loading.value = false
  }
}

async function openCreate() {
  mode.value = 'create'
  Object.assign(form, {
    username: '',
    display_name: '',
    password: '',
    role: 'viewer',
    is_active: true,
    system_username: '',
    create_panel_user: true,
    create_system_user: false,
    home_directory: '',
    shell: '/bin/bash',
    system_groups: [],
    allow_sudo: false,
    create_home: true,
    allow_ssh: false,
    ssh_public_key: '',
  })
  if (availableGroups.value.length === 0) await loadSystemGroups()
  editorVisible.value = true
}

function openEdit(user: PanelUser) {
  mode.value = 'edit'
  selected.value = user
  Object.assign(form, {
    username: user.username,
    display_name: user.display_name,
    password: '',
    role: user.role,
    is_active: user.is_active,
    system_username: user.system_username || '',
  })
  editorVisible.value = true
}

async function save() {
  saving.value = true
  try {
    if (mode.value === 'create') await api.post('/users', form)
    else await api.patch(`/users/${selected.value?.id}`, form)
    editorVisible.value = false
    toast.add({ severity: 'success', summary: t.value.saved, life: 3000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: t.value.operationFailed, life: 5000 })
  } finally {
    saving.value = false
  }
}

function openReset(user: PanelUser) {
  selected.value = user
  resetPassword.value = ''
  resetVisible.value = true
}

async function submitReset() {
  if (resetPassword.value.length < 12) return
  saving.value = true
  try {
    await api.post(`/users/${selected.value?.id}/reset-password`, { password: resetPassword.value })
    resetVisible.value = false
    toast.add({
      severity: 'success',
      summary: t.value.passwordReset,
      detail: t.value.sessionsRevoked,
      life: 5000,
    })
  } finally {
    saving.value = false
  }
}

async function showSessions(user: PanelUser) {
  selected.value = user
  systemDetails.value = undefined
  const webRequest = api.get<WebSession[]>(`/users/${user.id}/sessions`)
  const systemRequest = user.system_username
    ? api.get<SystemDetails>(`/users/${user.id}/system-details`)
    : Promise.resolve(null)
  const [webResult, systemResult] = await Promise.allSettled([webRequest, systemRequest])
  if (webResult.status === 'fulfilled') sessions.value = webResult.value.data
  if (systemResult.status === 'fulfilled' && systemResult.value)
    systemDetails.value = systemResult.value.data
  sessionsVisible.value = true
}

function remove(user: PanelUser) {
  selected.value = user
  Object.assign(deleteOptions, {
    delete_panel_user: true,
    delete_system_user: false,
    delete_home_directory: false,
    delete_ssh_keys: false,
    terminate_sessions: false,
  })
  deleteVisible.value = true
}

async function submitDelete() {
  if (!selected.value) return
  saving.value = true
  try {
    await api.delete(`/users/${selected.value.id}`, {
      data: {
        ...deleteOptions,
        delete_home_directory:
          deleteOptions.delete_system_user && deleteOptions.delete_home_directory,
      },
    })
    deleteVisible.value = false
    toast.add({ severity: 'success', summary: t.value.deleted, life: 3000 })
    await load()
  } finally {
    saving.value = false
  }
}

async function loadSystemGroups() {
  const response = await api.get<Array<{ groups: string[] }>>('/system-users')
  availableGroups.value = [...new Set(response.data.flatMap((item) => item.groups || []))].sort()
}

onMounted(load)
</script>

<template>
  <section>
    <div class="mb-5 flex flex-wrap items-center gap-3">
      <h1 class="text-2xl font-semibold">{{ t.users }}</h1>
      <span class="flex-1" />
      <Button
        v-if="authStore.role === 'admin'"
        :label="t.createUser"
        icon="pi pi-user-plus"
        @click="openCreate"
      />
    </div>
    <DataTable
      :value="users"
      :loading="loading"
      size="small"
      scrollable
      scroll-height="calc(100vh - 180px)"
      striped-rows
    >
      <Column field="username" header="Username" sortable />
      <Column field="display_name" :header="t.displayName" />
      <Column field="role" :header="t.role"
        ><template #body="{ data }"><Tag :value="data.role" /></template
      ></Column>
      <Column field="system_username" :header="t.ubuntuUser" />
      <Column field="created_at" :header="t.created"
        ><template #body="{ data }">{{ formatDateTime(data.created_at, locale) }}</template></Column
      >
      <Column :header="t.status"
        ><template #body="{ data }"
          ><Tag
            :severity="data.is_active ? 'success' : 'secondary'"
            :value="data.is_active ? t.active : t.disabled" /></template
      ></Column>
      <Column frozen align-frozen="right">
        <template #body="{ data }">
          <div class="flex justify-end gap-1">
            <Button
              v-if="authStore.role === 'admin'"
              icon="pi pi-pencil"
              text
              rounded
              :aria-label="t.edit"
              @click="openEdit(data)"
            />
            <Button
              v-if="authStore.role === 'admin'"
              icon="pi pi-key"
              text
              rounded
              :aria-label="t.resetPassword"
              @click="openReset(data)"
            />
            <Button
              icon="pi pi-desktop"
              text
              rounded
              :aria-label="t.sessions"
              @click="showSessions(data)"
            />
            <Button
              v-if="authStore.role === 'admin'"
              icon="pi pi-trash"
              text
              rounded
              severity="danger"
              :aria-label="t.delete"
              @click="remove(data)"
            />
          </div>
        </template>
      </Column>
    </DataTable>

    <Dialog
      v-model:visible="editorVisible"
      modal
      :header="mode === 'create' ? t.createUser : t.editUser"
      :style="{ width: 'min(52rem, calc(100vw - 2rem))' }"
    >
      <Fluid
        ><form
          id="user-editor"
          class="grid grid-cols-1 gap-6 pt-3 sm:grid-cols-2"
          @submit.prevent="save"
        >
          <div v-if="mode === 'create'" class="col-span-full flex flex-wrap gap-5">
            <label class="flex items-center gap-2">
              <Checkbox v-model="form.create_panel_user" binary />
              {{ t.createPanelUser }}
            </label>
            <label class="flex items-center gap-2">
              <Checkbox v-model="form.create_system_user" binary />
              {{ t.createSystemUser }}
            </label>
          </div>
          <FloatLabel variant="on"
            ><InputText
              id="panel-username"
              v-model="form.username"
              :disabled="mode === 'edit'"
            /><label for="panel-username">Username</label></FloatLabel
          >
          <FloatLabel variant="on"
            ><InputText id="display-name" v-model="form.display_name" /><label for="display-name">{{
              t.displayName
            }}</label></FloatLabel
          >
          <FloatLabel v-if="mode === 'create' && form.create_panel_user" variant="on"
            ><Password id="panel-password" v-model="form.password" toggle-mask /><label
              for="panel-password"
              >{{ t.password }}</label
            ></FloatLabel
          >
          <FloatLabel v-if="mode === 'edit' || form.create_panel_user" variant="on"
            ><Select id="panel-role" v-model="form.role" :options="roles" /><label
              for="panel-role"
              >{{ t.role }}</label
            ></FloatLabel
          >
          <FloatLabel variant="on"
            ><InputText id="system-username" v-model="form.system_username" /><label
              for="system-username"
              >{{ t.ubuntuUser }}</label
            ></FloatLabel
          >
          <template v-if="mode === 'create' && form.create_system_user">
            <FloatLabel variant="on">
              <InputText id="home-directory" v-model="form.home_directory" />
              <label for="home-directory">{{ t.homeDirectory }}</label>
            </FloatLabel>
            <FloatLabel variant="on">
              <Select
                id="system-shell"
                v-model="form.shell"
                :options="['/bin/bash', '/bin/sh', '/usr/sbin/nologin', '/bin/false']"
              />
              <label for="system-shell">Shell</label>
            </FloatLabel>
            <FloatLabel variant="on" class="col-span-full">
              <MultiSelect
                id="system-groups"
                v-model="form.system_groups"
                :options="availableGroups"
                filter
                display="chip"
              />
              <label for="system-groups">{{ t.systemGroups }}</label>
            </FloatLabel>
            <div class="col-span-full grid gap-3 sm:grid-cols-3">
              <label class="flex items-center gap-2">
                <Checkbox v-model="form.create_home" binary />{{ t.createHome }}
              </label>
              <label class="flex items-center gap-2">
                <Checkbox v-model="form.allow_sudo" binary />{{ t.allowSudo }}
              </label>
              <label class="flex items-center gap-2">
                <Checkbox v-model="form.allow_ssh" binary />{{ t.allowSSH }}
              </label>
            </div>
            <FloatLabel v-if="form.allow_ssh" variant="on" class="col-span-full">
              <Textarea id="ssh-public-key" v-model="form.ssh_public_key" rows="4" />
              <label for="ssh-public-key">{{ t.sshPublicKey }}</label>
            </FloatLabel>
          </template>
          <label v-if="mode === 'edit'" class="flex items-center gap-3"
            ><ToggleSwitch v-model="form.is_active" />{{ t.active }}</label
          >
        </form></Fluid
      >
      <template #footer
        ><Button
          :label="t.cancel"
          severity="secondary"
          text
          @click="editorVisible = false" /><Button
          type="submit"
          form="user-editor"
          :label="t.save"
          icon="pi pi-save"
          :loading="saving"
          :disabled="mode === 'create' && !form.create_panel_user && !form.create_system_user"
      /></template>
    </Dialog>

    <Dialog
      v-model:visible="deleteVisible"
      modal
      :header="`${t.deleteUser}: ${selected?.username || ''}`"
      :style="{ width: 'min(32rem, calc(100vw - 2rem))' }"
    >
      <div class="grid gap-4">
        <label class="flex items-center gap-3">
          <Checkbox v-model="deleteOptions.delete_panel_user" binary />{{ t.deletePanelUser }}
        </label>
        <label class="flex items-center gap-3">
          <Checkbox
            v-model="deleteOptions.delete_system_user"
            binary
            :disabled="!selected?.system_username"
          />{{ t.deleteSystemUser }}
        </label>
        <label class="flex items-center gap-3 pl-8">
          <Checkbox
            v-model="deleteOptions.delete_home_directory"
            binary
            :disabled="!deleteOptions.delete_system_user"
          />{{ t.deleteHomeDirectory }}
        </label>
        <label class="flex items-center gap-3">
          <Checkbox
            v-model="deleteOptions.delete_ssh_keys"
            binary
            :disabled="!selected?.system_username"
          />{{ t.deleteSSHKeys }}
        </label>
        <label class="flex items-center gap-3">
          <Checkbox v-model="deleteOptions.terminate_sessions" binary />{{ t.terminateSessions }}
        </label>
      </div>
      <template #footer>
        <Button :label="t.cancel" severity="secondary" text @click="deleteVisible = false" />
        <Button
          :label="t.delete"
          icon="pi pi-trash"
          severity="danger"
          :loading="saving"
          :disabled="
            !deleteOptions.delete_panel_user &&
            !deleteOptions.delete_system_user &&
            !deleteOptions.delete_ssh_keys &&
            !deleteOptions.terminate_sessions
          "
          @click="submitDelete"
        />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="resetVisible"
      modal
      :header="t.resetPassword"
      :style="{ width: '28rem' }"
    >
      <Fluid
        ><FloatLabel variant="on"
          ><Password id="reset-password" v-model="resetPassword" toggle-mask /><label
            for="reset-password"
            >{{ t.temporaryPassword }}</label
          ></FloatLabel
        ></Fluid
      >
      <template #footer
        ><Button :label="t.cancel" severity="secondary" text @click="resetVisible = false" /><Button
          :label="t.resetPassword"
          icon="pi pi-key"
          :loading="saving"
          :disabled="resetPassword.length < 12"
          @click="submitReset"
      /></template>
    </Dialog>

    <Dialog
      v-model:visible="sessionsVisible"
      modal
      :header="`${t.sessions}: ${selected?.username || ''}`"
      :style="{ width: '60rem' }"
    >
      <Tabs value="web">
        <TabList>
          <Tab value="web">{{ t.webSessions }}</Tab>
          <Tab v-if="selected?.system_username" value="system">{{ t.ubuntuDetails }}</Tab>
        </TabList>
        <TabPanels>
          <TabPanel value="web">
            <DataTable :value="sessions" size="small" scrollable>
              <Column field="ip_address" header="IP" />
              <Column field="user_agent" header="User Agent" />
              <Column :header="t.lastActivity">
                <template #body="{ data }">{{
                  formatDateTime(data.last_seen_at, locale)
                }}</template>
              </Column>
              <Column :header="t.status">
                <template #body="{ data }">
                  <Tag
                    :severity="data.revoked_at ? 'secondary' : 'success'"
                    :value="data.revoked_at ? t.revoked : t.active"
                  />
                </template>
              </Column>
            </DataTable>
          </TabPanel>
          <TabPanel v-if="selected?.system_username" value="system">
            <div v-if="systemDetails" class="grid gap-5">
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <div>
                  <span class="text-muted-color">UID / GID</span><br />{{ systemDetails.uid }} /
                  {{ systemDetails.gid }}
                </div>
                <div>
                  <span class="text-muted-color">Shell</span><br />{{ systemDetails.shell }}
                </div>
                <div>
                  <span class="text-muted-color">{{ t.sudo }}</span
                  ><br /><Tag
                    :severity="systemDetails.has_sudo ? 'success' : 'secondary'"
                    :value="systemDetails.has_sudo ? t.allowed : t.notAllowed"
                  />
                </div>
                <div>
                  <span class="text-muted-color">SSH</span><br /><Tag
                    :severity="systemDetails.has_ssh_keys ? 'success' : 'secondary'"
                    :value="systemDetails.has_ssh_keys ? t.keysConfigured : t.noKeys"
                  />
                </div>
                <div class="sm:col-span-2">
                  <span class="text-muted-color">{{ t.homeDirectory }}</span
                  ><br />{{ systemDetails.home }}
                </div>
                <div class="sm:col-span-2">
                  <span class="text-muted-color">{{ t.lastUbuntuLogin }}</span
                  ><br />{{ formatDateTime(systemDetails.last_login_at, locale) }}
                </div>
              </div>
              <div class="flex flex-wrap gap-2">
                <Tag
                  v-for="group in systemDetails.groups"
                  :key="group"
                  :value="group"
                  severity="secondary"
                />
              </div>
              <DataTable
                :value="systemDetails.active_sessions"
                size="small"
                :empty-message="t.noActiveSessions"
              >
                <Column field="terminal" :header="t.terminal" />
                <Column field="remote_ip" header="IP" />
                <Column :header="t.startedAt"
                  ><template #body="{ data }">{{
                    formatDateTime(data.started_at, locale)
                  }}</template></Column
                >
              </DataTable>
            </div>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </Dialog>
  </section>
</template>
