<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import Tag from 'primevue/tag'
import api from '../services/api'
import { useI18n } from '../services/i18n'

const { t } = useI18n()
interface Connection {
  engine: string
  host: string
  port: number
  database: string
  username: string
  password: string
  auth_source: string
  tls: boolean
}
interface Result {
  tables: string[]
  columns: string[]
  rows: Record<string, unknown>[]
  has_more: boolean
}
const form = reactive<Connection>({
  engine: 'postgres',
  host: '127.0.0.1',
  port: 5432,
  database: '',
  username: '',
  password: '',
  auth_source: 'admin',
  tls: false,
})
const engines = [
  { label: 'PostgreSQL', value: 'postgres' },
  { label: 'MySQL / MariaDB', value: 'mysql' },
  { label: 'MongoDB', value: 'mongodb' },
]
const ports: Record<string, number> = { postgres: 5432, mysql: 3306, mongodb: 27017 }
const connection = ref<Connection>()
const result = ref<Result>({ tables: [], columns: [], rows: [], has_more: false })
const table = ref('')
const offset = ref(0)
const loading = ref(false)
const failed = ref(false)
const connected = computed(() => Boolean(connection.value))
let generation = 0

function disconnect() {
  generation++
  connection.value = undefined
  form.password = ''
  table.value = ''
  offset.value = 0
  result.value = { tables: [], columns: [], rows: [], has_more: false }
  failed.value = false
}

async function browse(nextOffset = 0, connect = false) {
  if (loading.value) return
  loading.value = true
  failed.value = false
  const current = ++generation
  const credentials = connect ? { ...form } : connection.value
  if (!credentials) {
    loading.value = false
    return
  }
  try {
    const response = await api.post<Result>('/databases/browse', {
      ...credentials,
      table: connect ? '' : table.value,
      offset: nextOffset,
    })
    if (current !== generation) return
    result.value = response.data
    offset.value = nextOffset
    if (connect) {
      connection.value = credentials
      form.password = ''
    }
  } catch {
    if (current === generation) {
      failed.value = true
      result.value.rows = []
      result.value.has_more = false
    }
  } finally {
    loading.value = false
  }
}

function cell(value: unknown) {
  if (value === null) return 'NULL'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value ?? '')
}
onBeforeUnmount(disconnect)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-2xl font-semibold">{{ t.databases }}</h1>
        <p class="muted mt-1 text-sm">{{ t.databaseHint }}</p>
      </div>
      <Tag
        :value="connected ? t.databaseConnected : t.databaseDisconnected"
        :severity="connected ? 'success' : 'secondary'"
      />
    </div>
    <Message severity="info" :closable="false">{{ t.databaseSecurityHint }}</Message>
    <Card v-if="!connected">
      <template #title>{{ t.databaseConnect }}</template>
      <template #content>
        <form class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3" @submit.prevent="browse(0, true)">
          <div class="flex flex-col gap-2">
            <label for="db-engine">{{ t.databaseEngine }}</label
            ><Select
              v-model="form.engine"
              input-id="db-engine"
              :aria-label="t.databaseEngine"
              :options="engines"
              option-label="label"
              option-value="value"
              @change="form.port = ports[form.engine] || 5432"
            />
          </div>
          <div class="flex flex-col gap-2">
            <label for="db-host">{{ t.databaseHost }}</label
            ><InputText id="db-host" v-model="form.host" required :maxlength="253" />
          </div>
          <div class="flex flex-col gap-2">
            <label for="db-port">{{ t.hostPort }}</label
            ><InputNumber
              v-model="form.port"
              input-id="db-port"
              :min="1"
              :max="65535"
              :use-grouping="false"
              required
            />
          </div>
          <div class="flex flex-col gap-2">
            <label for="db-name">{{ t.databaseName }}</label
            ><InputText
              id="db-name"
              v-model="form.database"
              required
              pattern="[A-Za-z_][A-Za-z0-9_]{0,127}"
            />
          </div>
          <div class="flex flex-col gap-2">
            <label for="db-user">{{ t.username }}</label
            ><InputText id="db-user" v-model="form.username" autocomplete="off" :maxlength="256" />
          </div>
          <div class="flex flex-col gap-2">
            <label for="db-password">{{ t.password }}</label
            ><Password
              v-model="form.password"
              input-id="db-password"
              :feedback="false"
              toggle-mask
              autocomplete="off"
              fluid
            />
          </div>
          <div v-if="form.engine === 'mongodb'" class="flex flex-col gap-2">
            <label for="db-auth">{{ t.databaseAuthSource }}</label
            ><InputText id="db-auth" v-model="form.auth_source" />
          </div>
          <div class="flex items-center gap-2">
            <Checkbox v-model="form.tls" input-id="db-tls" binary /><label for="db-tls">{{
              t.databaseTLS
            }}</label>
          </div>
          <div class="flex items-end justify-end sm:col-span-2 xl:col-span-3">
            <Button
              type="submit"
              :label="t.databaseConnect"
              icon="pi pi-database"
              :loading="loading"
            />
          </div>
        </form>
      </template>
    </Card>
    <template v-else>
      <div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
        <div class="min-w-0 flex-1">
          <p class="muted mb-2 break-all text-sm">
            {{ connection?.engine }} · {{ connection?.host }}:{{ connection?.port }} /
            {{ connection?.database }}
          </p>
          <label class="mb-2 block" for="db-table">{{ t.databaseTable }}</label
          ><Select
            v-model="table"
            input-id="db-table"
            :aria-label="t.databaseTable"
            :options="result.tables"
            filter
            class="w-full sm:max-w-md"
            :disabled="loading"
            :placeholder="t.databaseChooseTable"
            @change="browse(0)"
          />
        </div>
        <Button
          :label="t.refresh"
          icon="pi pi-refresh"
          severity="secondary"
          :loading="loading"
          @click="browse(offset)"
        />
        <Button :label="t.disconnect" severity="secondary" @click="disconnect" />
      </div>
      <Message
        v-if="!result.tables.length && !loading && !failed"
        severity="info"
        :closable="false"
        >{{ t.databaseNoTables }}</Message
      >
      <template v-if="table">
        <DataTable
          :value="result.rows"
          :loading="loading"
          size="small"
          scrollable
          scroll-height="55vh"
          :virtual-scroller-options="{ itemSize: 40 }"
          striped-rows
        >
          <template #empty>{{ failed ? t.databaseFailed : t.databaseNoRows }}</template>
          <Column v-for="column in result.columns" :key="column" :field="column" :header="column"
            ><template #body="{ data }"
              ><span
                class="block max-w-96 truncate font-mono text-xs"
                :title="cell(data[column])"
                >{{ cell(data[column]) }}</span
              ></template
            ></Column
          >
        </DataTable>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="muted text-sm"
            >{{ t.databaseRows }}: {{ result.rows.length ? offset + 1 : 0 }}–{{
              offset + result.rows.length
            }}</span
          >
          <div class="flex gap-2">
            <Button
              :label="t.databasePrevious"
              severity="secondary"
              :disabled="loading || offset === 0"
              @click="browse(Math.max(0, offset - 100))"
            /><Button
              :label="t.databaseNext"
              severity="secondary"
              :disabled="loading || !result.has_more || offset >= 100000"
              @click="browse(offset + 100)"
            />
          </div>
        </div>
      </template>
    </template>
    <Message v-if="failed" severity="error" :closable="false">{{ t.databaseFailed }}</Message>
  </section>
</template>
