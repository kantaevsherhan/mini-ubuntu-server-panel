<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import api from '../services/api'
import { useI18n } from '../services/i18n'

interface Recipient {
  id?: number
  telegram_user_id: number | null
  telegram_chat_id: number
  display_name: string
  enabled: boolean
  receive_alerts: boolean
  receive_audit: boolean
  receive_updates: boolean
}
interface Candidate {
  telegram_user_id: number
  telegram_chat_id: number
  display_name: string
  username: string
  chat_type: string
}

const settings = reactive({
  enabled: false,
  api_base_url: 'https://api.telegram.org',
  request_timeout_seconds: 10,
  retry_count: 3,
  token_configured: false,
})
const recipients = ref<Recipient[]>([]),
  candidates = ref<Candidate[]>([]),
  loading = ref(true),
  saving = ref(false),
  editorVisible = ref(false)
const connection = ref<{ username: string; recipients: number }>()
const botToken = ref('')
const editor = reactive<Recipient>({
  telegram_user_id: null,
  telegram_chat_id: 0,
  display_name: '',
  enabled: true,
  receive_alerts: true,
  receive_audit: false,
  receive_updates: true,
})
const confirm = useConfirm(),
  toast = useToast(),
  { t } = useI18n()

async function load() {
  loading.value = true
  try {
    Object.assign(settings, (await api.get('/telegram/settings')).data)
    recipients.value = (await api.get('/telegram/recipients')).data
  } finally {
    loading.value = false
  }
}
async function saveSettings() {
  saving.value = true
  try {
    await api.put('/telegram/settings', settings)
    toast.add({ severity: 'success', summary: t.value.saved, life: 3000 })
  } finally {
    saving.value = false
  }
}
async function saveToken() {
  saving.value = true
  try {
    await api.put('/telegram/token', { token: botToken.value })
    botToken.value = ''
    settings.token_configured = true
    toast.add({ severity: 'success', summary: t.value.telegramTokenUpdated, life: 3000 })
  } finally {
    saving.value = false
  }
}
async function checkConnection() {
  connection.value = (await api.post('/telegram/check')).data
  toast.add({
    severity: 'success',
    summary: t.value.telegramConnected,
    detail: `@${connection.value?.username}`,
    life: 5000,
  })
}
async function getUpdates() {
  candidates.value = (await api.get('/telegram/updates')).data
}
function openRecipient(value?: Partial<Recipient>) {
  Object.assign(
    editor,
    {
      telegram_user_id: null,
      telegram_chat_id: 0,
      display_name: '',
      enabled: true,
      receive_alerts: true,
      receive_audit: false,
      receive_updates: true,
    },
    value,
  )
  editorVisible.value = true
}
async function saveRecipient() {
  saving.value = true
  try {
    if (editor.id) await api.put(`/telegram/recipients/${editor.id}`, editor)
    else await api.post('/telegram/recipients', editor)
    editorVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}
async function testRecipient(value: Recipient) {
  await api.post(`/telegram/recipients/${value.id}/test`)
  toast.add({ severity: 'success', summary: t.value.testMessageSent, life: 4000 })
}
function removeRecipient(value: Recipient) {
  confirm.require({
    header: t.value.delete,
    message: `${t.value.deleteRecipient} ${value.display_name}?`,
    acceptProps: { label: t.value.delete, severity: 'danger' },
    rejectProps: { label: t.value.cancel, severity: 'secondary', outlined: true },
    accept: async () => {
      await api.delete(`/telegram/recipients/${value.id}`)
      await load()
    },
  })
}
onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <Card
      ><template #title>{{ t.telegramConfiguration }}</template
      ><template #content>
        <Message severity="info" class="mb-5">{{ t.telegramTokenHint }}</Message>
        <Fluid
          ><div class="grid grid-cols-1 gap-6 md:grid-cols-2">
            <label class="flex items-center gap-3"
              ><ToggleSwitch v-model="settings.enabled" />{{ t.telegramEnabled }}</label
            >
            <div class="grid gap-3">
              <span class="muted text-sm">Bot Token</span>
              <div>
                <Tag
                  :severity="settings.token_configured ? 'success' : 'warn'"
                  :value="settings.token_configured ? t.configuredHidden : t.notConfigured"
                />
              </div>
              <Password
                v-model="botToken"
                :feedback="false"
                toggle-mask
                autocomplete="new-password"
                :placeholder="t.telegramTokenPlaceholder"
              />
              <Button
                :label="t.updateTelegramToken"
                icon="pi pi-key"
                severity="secondary"
                :disabled="botToken.length < 37"
                :loading="saving"
                @click="saveToken"
              />
            </div>
            <FloatLabel variant="on"
              ><InputText id="telegram-url" v-model="settings.api_base_url" /><label
                for="telegram-url"
                >Bot API URL</label
              ></FloatLabel
            >
            <FloatLabel variant="on"
              ><InputNumber
                id="telegram-timeout"
                v-model="settings.request_timeout_seconds"
                :min="1"
                :max="60"
              /><label for="telegram-timeout">{{ t.requestTimeout }}</label></FloatLabel
            >
            <FloatLabel variant="on"
              ><InputNumber
                id="telegram-retries"
                v-model="settings.retry_count"
                :min="0"
                :max="10"
              /><label for="telegram-retries">{{ t.retryCount }}</label></FloatLabel
            >
          </div></Fluid
        >
        <div class="mt-5 flex flex-wrap gap-2">
          <Button
            :label="t.save"
            icon="pi pi-save"
            :loading="saving"
            @click="saveSettings"
          /><Button
            :label="t.checkConnection"
            icon="pi pi-check-circle"
            severity="secondary"
            @click="checkConnection"
          /><Button
            :label="t.getUpdates"
            icon="pi pi-download"
            severity="secondary"
            @click="getUpdates"
          />
        </div> </template
    ></Card>

    <Card v-if="candidates.length"
      ><template #title>{{ t.foundTelegramChats }}</template
      ><template #content
        ><DataTable :value="candidates" size="small"
          ><Column field="telegram_user_id" header="User ID" /><Column
            field="telegram_chat_id"
            header="Chat ID" /><Column field="display_name" :header="t.displayName" /><Column
            field="chat_type"
            :header="t.chatType" /><Column
            ><template #body="{ data }"
              ><Button
                :label="t.add"
                icon="pi pi-plus"
                size="small"
                @click="openRecipient(data)" /></template></Column></DataTable></template
    ></Card>

    <Card
      ><template #title
        ><div class="flex items-center">
          <span>{{ t.telegramRecipients }}</span
          ><Button
            :label="t.add"
            icon="pi pi-plus"
            class="ml-auto"
            size="small"
            @click="openRecipient()"
          /></div></template
      ><template #content
        ><DataTable :value="recipients" :loading="loading" size="small" scrollable
          ><Column field="telegram_user_id" header="User ID" /><Column
            field="telegram_chat_id"
            header="Chat ID" /><Column field="display_name" :header="t.displayName" /><Column
            :header="t.status"
            ><template #body="{ data }"
              ><Tag
                :severity="data.enabled ? 'success' : 'secondary'"
                :value="data.enabled ? t.active : t.disabled" /></template></Column
          ><Column
            ><template #body="{ data }"
              ><div class="flex justify-end gap-1">
                <Button
                  icon="pi pi-send"
                  text
                  rounded
                  :aria-label="t.sendTest"
                  @click="testRecipient(data)"
                /><Button
                  icon="pi pi-pencil"
                  text
                  rounded
                  :aria-label="t.edit"
                  @click="openRecipient(data)"
                /><Button
                  icon="pi pi-trash"
                  text
                  rounded
                  severity="danger"
                  :aria-label="t.delete"
                  @click="removeRecipient(data)"
                /></div></template></Column></DataTable></template
    ></Card>

    <Dialog
      v-model:visible="editorVisible"
      modal
      :header="t.telegramRecipient"
      :style="{ width: '34rem' }"
      ><Fluid
        ><form
          id="recipient-form"
          class="grid grid-cols-1 gap-6 pt-3 sm:grid-cols-2"
          @submit.prevent="saveRecipient"
        >
          <FloatLabel variant="on"
            ><InputNumber
              id="telegram-user-id"
              v-model="editor.telegram_user_id"
              :use-grouping="false"
            /><label for="telegram-user-id">User ID</label></FloatLabel
          ><FloatLabel variant="on"
            ><InputNumber
              id="telegram-chat-id"
              v-model="editor.telegram_chat_id"
              :use-grouping="false"
            /><label for="telegram-chat-id">Chat ID</label></FloatLabel
          ><FloatLabel variant="on" class="sm:col-span-2"
            ><InputText id="telegram-name" v-model="editor.display_name" /><label
              for="telegram-name"
              >{{ t.displayName }}</label
            ></FloatLabel
          ><label class="flex items-center gap-2"
            ><ToggleSwitch v-model="editor.enabled" />{{ t.active }}</label
          ><label class="flex items-center gap-2"
            ><ToggleSwitch v-model="editor.receive_alerts" />{{ t.receiveAlerts }}</label
          ><label class="flex items-center gap-2"
            ><ToggleSwitch v-model="editor.receive_audit" />{{ t.receiveAudit }}</label
          ><label class="flex items-center gap-2"
            ><ToggleSwitch v-model="editor.receive_updates" />{{ t.receiveUpdates }}</label
          >
        </form></Fluid
      ><template #footer
        ><Button
          :label="t.cancel"
          text
          severity="secondary"
          @click="editorVisible = false" /><Button
          type="submit"
          form="recipient-form"
          :label="t.save"
          icon="pi pi-save"
          :loading="saving" /></template
    ></Dialog>
  </div>
</template>
