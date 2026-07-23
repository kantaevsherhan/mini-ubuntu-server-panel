<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import FloatLabel from 'primevue/floatlabel'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import api from '../services/api'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

interface FirewallRule {
  number: number
  to: string
  action: string
  direction: string
  from: string
}

interface FirewallStatus {
  active: boolean
  rules: FirewallRule[]
}

const status = ref<FirewallStatus>({ active: false, rules: [] })
const loading = ref(false)
const saving = ref(false)
const editorVisible = ref(false)
const form = reactive({ action: 'allow', port: 80, protocol: 'tcp', source: 'any' })
const actions = ['allow', 'deny']
const protocols = ['tcp', 'udp']
const auth = useAuthStore()
const confirm = useConfirm()
const toast = useToast()
const { t } = useI18n()
const canManage = computed(() => auth.role === 'admin')

async function load() {
  loading.value = true
  try {
    status.value = (await api.get<FirewallStatus>('/firewall')).data
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, { action: 'allow', port: 80, protocol: 'tcp', source: 'any' })
  editorVisible.value = true
}

async function addRule() {
  saving.value = true
  try {
    await api.post('/firewall/rules', form)
    editorVisible.value = false
    toast.add({ severity: 'success', summary: t.value.firewallRuleAdded, life: 3000 })
    await load()
  } finally {
    saving.value = false
  }
}

function removeRule(rule: FirewallRule) {
  confirm.require({
    header: t.value.deleteFirewallRule,
    message: `${t.value.deleteFirewallRuleConfirm}: #${rule.number} ${rule.to} ${rule.action}`,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.delete,
    acceptClass: 'p-button-danger',
    accept: async () => {
      await api.delete(`/firewall/rules/${rule.number}`)
      toast.add({ severity: 'success', summary: t.value.deleted, life: 3000 })
      await load()
    },
  })
}

function actionSeverity(action: string) {
  if (action === 'allow') return 'success'
  if (action === 'deny' || action === 'reject') return 'danger'
  return 'secondary'
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <div class="flex items-center gap-3">
          <h1 class="text-2xl font-semibold">{{ t.firewall }}</h1>
          <Tag
            :value="status.active ? t.active : t.disabled"
            :severity="status.active ? 'success' : 'secondary'"
          />
        </div>
        <p class="muted mt-1 text-sm">{{ t.firewallHint }}</p>
      </div>
      <div class="flex gap-2">
        <Button
          :label="t.refresh"
          icon="pi pi-refresh"
          :loading="loading"
          severity="secondary"
          @click="load"
        />
        <Button v-if="canManage" :label="t.addFirewallRule" icon="pi pi-plus" @click="openCreate" />
      </div>
    </div>

    <DataTable :value="status.rules" :loading="loading" size="small" striped-rows data-key="number">
      <Column field="number" header="#" sortable class="w-20" />
      <Column field="to" :header="t.destination" sortable />
      <Column field="action" :header="t.action" sortable class="w-32">
        <template #body="{ data }"
          ><Tag :value="data.action" :severity="actionSeverity(data.action)"
        /></template>
      </Column>
      <Column field="direction" :header="t.direction" sortable class="w-28" />
      <Column field="from" :header="t.source" sortable />
      <Column v-if="canManage" :header="t.actions" class="w-24">
        <template #body="{ data }"
          ><Button
            icon="pi pi-trash"
            text
            severity="danger"
            :aria-label="t.delete"
            @click="removeRule(data)"
        /></template>
      </Column>
      <template #empty>{{ t.noFirewallRules }}</template>
    </DataTable>

    <Dialog
      v-model:visible="editorVisible"
      modal
      :header="t.addFirewallRule"
      class="w-[min(32rem,calc(100vw-2rem))]"
    >
      <form class="space-y-5 pt-3" @submit.prevent="addRule">
        <div class="grid grid-cols-1 gap-5 sm:grid-cols-2">
          <FloatLabel variant="on"
            ><Select
              v-model="form.action"
              input-id="firewall-action"
              :options="actions"
              class="w-full"
            /><label for="firewall-action">{{ t.action }}</label></FloatLabel
          >
          <FloatLabel variant="on"
            ><Select
              v-model="form.protocol"
              input-id="firewall-protocol"
              :options="protocols"
              class="w-full"
            /><label for="firewall-protocol">{{ t.protocol }}</label></FloatLabel
          >
        </div>
        <FloatLabel variant="on"
          ><InputNumber
            v-model="form.port"
            input-id="firewall-port"
            :min="1"
            :max="65535"
            :use-grouping="false"
            class="w-full"
          /><label for="firewall-port">{{ t.port }}</label></FloatLabel
        >
        <FloatLabel variant="on"
          ><InputText
            id="firewall-source"
            v-model="form.source"
            maxlength="64"
            class="w-full"
          /><label for="firewall-source">{{ t.sourceCIDR }}</label></FloatLabel
        >
        <small class="muted block">{{ t.firewallSSHProtection }}</small>
        <div class="flex justify-end gap-2">
          <Button
            :label="t.cancel"
            severity="secondary"
            text
            type="button"
            @click="editorVisible = false"
          /><Button :label="t.add" icon="pi pi-plus" type="submit" :loading="saving" />
        </div>
      </form>
    </Dialog>
  </section>
</template>
