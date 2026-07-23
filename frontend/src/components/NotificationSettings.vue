<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import InputNumber from 'primevue/inputnumber'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { notificationEventLabel, useI18n } from '../services/i18n'

const props = withDefaults(defineProps<{ readOnly?: boolean }>(), { readOnly: false })

interface Rule {
  event_key: string
  enabled: boolean
  severity: 'info' | 'warning' | 'error' | 'critical'
  recipient_ids: number[]
  cooldown_seconds: number
  repeat_interval_seconds: number
  send_recovery: boolean
  updated_at: string
}
interface Recipient {
  id: number
  display_name: string
  telegram_chat_id: number
}
interface Delivery {
  id: number
  recipient_id: number
  status: string
  attempts: number
  last_error: string
}
interface HistoryEvent {
  id: number
  event_key: string
  severity: string
  status: string
  created_at: string
  resolved_at: string | null
  deliveries: Delivery[]
}

const rules = ref<Rule[]>([])
const recipients = ref<Recipient[]>([])
const history = ref<HistoryEvent[]>([])
const loading = ref(true)
const saving = ref(false)
const editorVisible = ref(false)
const selectedKey = ref('')
const editor = reactive({
  enabled: true,
  severity: 'warning' as Rule['severity'],
  recipient_ids: [] as number[],
  cooldown_minutes: 10,
  repeat_minutes: 30,
  send_recovery: true,
})
const severities = ['info', 'warning', 'error', 'critical']
const toast = useToast()
const { t, locale } = useI18n()

async function load() {
  loading.value = true
  try {
    if (props.readOnly) {
      history.value = (await api.get<HistoryEvent[]>('/notifications/history')).data
      return
    }
    const [rulesResponse, recipientsResponse, historyResponse] = await Promise.all([
      api.get<Rule[]>('/notifications/rules'),
      api.get<Recipient[]>('/telegram/recipients'),
      api.get<HistoryEvent[]>('/notifications/history'),
    ])
    rules.value = rulesResponse.data
    recipients.value = recipientsResponse.data
    history.value = historyResponse.data
  } finally {
    loading.value = false
  }
}

function openEditor(rule: Rule) {
  selectedKey.value = rule.event_key
  Object.assign(editor, {
    enabled: rule.enabled,
    severity: rule.severity,
    recipient_ids: [...(rule.recipient_ids || [])],
    cooldown_minutes: Math.round(rule.cooldown_seconds / 60),
    repeat_minutes: Math.round(rule.repeat_interval_seconds / 60),
    send_recovery: rule.send_recovery,
  })
  editorVisible.value = true
}

async function saveRule() {
  saving.value = true
  try {
    await api.put(`/notifications/rules/${selectedKey.value}`, {
      enabled: editor.enabled,
      severity: editor.severity,
      recipient_ids: editor.recipient_ids,
      cooldown_seconds: editor.cooldown_minutes * 60,
      repeat_interval_seconds: editor.repeat_minutes * 60,
      send_recovery: editor.send_recovery,
    })
    editorVisible.value = false
    toast.add({ severity: 'success', summary: t.value.saved, life: 3000 })
    await load()
  } finally {
    saving.value = false
  }
}

function severityTag(severity: string) {
  if (severity === 'critical' || severity === 'error') return 'danger'
  if (severity === 'warning') return 'warn'
  return 'info'
}

function deliverySummary(event: HistoryEvent) {
  if (!event.deliveries.length) return t.value.noRecipients
  const delivered = event.deliveries.filter((item) => item.status === 'delivered').length
  return `${delivered}/${event.deliveries.length}`
}

onMounted(load)
</script>

<template>
  <div class="grid gap-4">
    <Card v-if="!props.readOnly">
      <template #title>{{ t.notificationRules }}</template>
      <template #content>
        <DataTable
          :value="rules"
          :loading="loading"
          size="small"
          scrollable
          scroll-height="32rem"
          :virtual-scroller-options="{ itemSize: 48 }"
          striped-rows
        >
          <Column field="event_key" :header="t.event">
            <template #body="{ data }">
              <div class="font-medium">{{ notificationEventLabel(data.event_key, locale) }}</div>
              <div class="text-muted-color text-xs">{{ data.event_key }}</div>
            </template>
          </Column>
          <Column :header="t.status">
            <template #body="{ data }">
              <Tag
                :severity="data.enabled ? 'success' : 'secondary'"
                :value="data.enabled ? t.active : t.disabled"
              />
            </template>
          </Column>
          <Column :header="t.severity">
            <template #body="{ data }">
              <Tag :severity="severityTag(data.severity)" :value="data.severity" />
            </template>
          </Column>
          <Column :header="t.recipients">
            <template #body="{ data }">{{
              data.recipient_ids?.length || t.allRecipients
            }}</template>
          </Column>
          <Column :header="t.cooldown">
            <template #body="{ data }"
              >{{ Math.round(data.cooldown_seconds / 60) }} {{ t.minutesShort }}</template
            >
          </Column>
          <Column :header="t.repeatInterval">
            <template #body="{ data }"
              >{{ Math.round(data.repeat_interval_seconds / 60) }} {{ t.minutesShort }}</template
            >
          </Column>
          <Column frozen align-frozen="right">
            <template #body="{ data }">
              <Button
                icon="pi pi-pencil"
                text
                rounded
                :aria-label="t.edit"
                @click="openEditor(data)"
              />
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Card>
      <template #title>{{ t.deliveryHistory }}</template>
      <template #content>
        <DataTable
          :value="history"
          :loading="loading"
          size="small"
          scrollable
          scroll-height="24rem"
          :virtual-scroller-options="{ itemSize: 46 }"
        >
          <Column :header="t.event"
            ><template #body="{ data }">{{
              notificationEventLabel(data.event_key, locale)
            }}</template></Column
          >
          <Column :header="t.severity"
            ><template #body="{ data }"
              ><Tag :severity="severityTag(data.severity)" :value="data.severity" /></template
          ></Column>
          <Column :header="t.status"
            ><template #body="{ data }"><Tag :value="data.status" /></template
          ></Column>
          <Column :header="t.deliveries"
            ><template #body="{ data }">{{ deliverySummary(data) }}</template></Column
          >
          <Column :header="t.created"
            ><template #body="{ data }">{{
              formatDateTime(data.created_at, locale)
            }}</template></Column
          >
        </DataTable>
      </template>
    </Card>

    <Dialog
      v-if="!props.readOnly"
      v-model:visible="editorVisible"
      modal
      :header="notificationEventLabel(selectedKey, locale)"
      :style="{ width: 'min(42rem, calc(100vw - 2rem))' }"
    >
      <Fluid>
        <div class="grid gap-6 pt-2 sm:grid-cols-2">
          <label class="flex items-center gap-3"
            ><ToggleSwitch v-model="editor.enabled" />{{ t.enabled }}</label
          >
          <label class="flex items-center gap-3"
            ><ToggleSwitch v-model="editor.send_recovery" />{{ t.sendRecovery }}</label
          >
          <FloatLabel variant="on"
            ><Select id="rule-severity" v-model="editor.severity" :options="severities" /><label
              for="rule-severity"
              >{{ t.severity }}</label
            ></FloatLabel
          >
          <FloatLabel variant="on"
            ><MultiSelect
              id="rule-recipients"
              v-model="editor.recipient_ids"
              :options="recipients"
              option-label="display_name"
              option-value="id"
              display="chip"
            /><label for="rule-recipients">{{ t.recipients }}</label></FloatLabel
          >
          <FloatLabel variant="on"
            ><InputNumber
              id="rule-cooldown"
              v-model="editor.cooldown_minutes"
              :min="0"
              :max="10080"
            /><label for="rule-cooldown">{{ t.cooldownMinutes }}</label></FloatLabel
          >
          <FloatLabel variant="on"
            ><InputNumber
              id="rule-repeat"
              v-model="editor.repeat_minutes"
              :min="0"
              :max="43200"
            /><label for="rule-repeat">{{ t.repeatMinutes }}</label></FloatLabel
          >
        </div>
      </Fluid>
      <template #footer
        ><Button
          :label="t.cancel"
          severity="secondary"
          text
          @click="editorVisible = false" /><Button
          :label="t.save"
          icon="pi pi-save"
          :loading="saving"
          @click="saveRule"
      /></template>
    </Dialog>
  </div>
</template>
