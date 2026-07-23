<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'

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

const users = ref<PanelUser[]>([])
const sessions = ref<WebSession[]>([])
const loading = ref(true)
const saving = ref(false)
const editorVisible = ref(false)
const resetVisible = ref(false)
const sessionsVisible = ref(false)
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
})
const roles = ['admin', 'operator', 'viewer']
const confirm = useConfirm()
const toast = useToast()
const { t, locale } = useI18n()

async function load() {
  loading.value = true
  try {
    users.value = (await api.get('/users')).data
  } finally {
    loading.value = false
  }
}

function openCreate() {
  mode.value = 'create'
  Object.assign(form, {
    username: '',
    display_name: '',
    password: '',
    role: 'viewer',
    is_active: true,
    system_username: '',
  })
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
  sessions.value = (await api.get(`/users/${user.id}/sessions`)).data
  sessionsVisible.value = true
}

function remove(user: PanelUser) {
  confirm.require({
    header: t.value.deleteUser,
    message: `${t.value.deleteUserConfirm} ${user.username}?`,
    icon: 'pi pi-exclamation-triangle',
    rejectProps: { label: t.value.cancel, severity: 'secondary', outlined: true },
    acceptProps: { label: t.value.delete, severity: 'danger' },
    accept: async () => {
      try {
        await api.delete(`/users/${user.id}`)
        toast.add({ severity: 'success', summary: t.value.deleted, life: 3000 })
        await load()
      } catch {
        toast.add({
          severity: 'error',
          summary: t.value.operationFailed,
          detail: t.value.lastAdminHint,
          life: 5000,
        })
      }
    },
  })
}

onMounted(load)
</script>

<template>
  <section>
    <div class="mb-5 flex flex-wrap items-center gap-3">
      <h1 class="text-2xl font-semibold">{{ t.users }}</h1>
      <span class="flex-1" />
      <Button :label="t.createUser" icon="pi pi-user-plus" @click="openCreate" />
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
            <Button icon="pi pi-pencil" text rounded :aria-label="t.edit" @click="openEdit(data)" />
            <Button
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
      :style="{ width: '36rem' }"
    >
      <Fluid
        ><form
          id="user-editor"
          class="grid grid-cols-1 gap-6 pt-3 sm:grid-cols-2"
          @submit.prevent="save"
        >
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
          <FloatLabel v-if="mode === 'create'" variant="on"
            ><Password id="panel-password" v-model="form.password" toggle-mask /><label
              for="panel-password"
              >{{ t.password }}</label
            ></FloatLabel
          >
          <FloatLabel variant="on"
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
      /></template>
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
      <DataTable :value="sessions" size="small" scrollable
        ><Column field="ip_address" header="IP" /><Column
          field="user_agent"
          header="User Agent" /><Column :header="t.lastActivity"
          ><template #body="{ data }">{{
            formatDateTime(data.last_seen_at, locale)
          }}</template></Column
        ><Column :header="t.status"
          ><template #body="{ data }"
            ><Tag
              :severity="data.revoked_at ? 'secondary' : 'success'"
              :value="data.revoked_at ? t.revoked : t.active" /></template></Column
      ></DataTable>
    </Dialog>
  </section>
</template>
