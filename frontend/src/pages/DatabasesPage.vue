<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Skeleton from 'primevue/skeleton'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import DbConnect from '../components/database/DbConnect.vue'
import DbData from '../components/database/DbData.vue'
import DbSql from '../components/database/DbSql.vue'
import DbStructure from '../components/database/DbStructure.vue'
import {
  databaseClient,
  engineLabels,
  formatBytes,
  saveRecent,
  type Connection,
  type Overview,
  type Structure,
} from '../services/databaseClient'
import { useI18n } from '../services/i18n'

const { t } = useI18n()
const sessionKey = 'database-session'

const connection = ref<Connection>()
const overview = ref<Overview>()
const connecting = ref(false)
const connectError = ref('')
const overviewLoading = ref(false)
const table = ref('')
const view = ref<'data' | 'structure' | 'sql'>('data')
const structure = ref<Structure>()
const structureTable = ref('')
const structureLoading = ref(false)
const structureError = ref('')
const tableFilter = ref('')
const sqlOnly = ref(false)

const errorLabels = computed<Record<string, string>>(() => ({
  database_connection_failed: t.value.dbErrorConnection,
  database_request_invalid: t.value.dbErrorInvalid,
  database_busy: t.value.dbErrorBusy,
  database_result_too_large: t.value.dbErrorTooLarge,
  database_no_primary_key: t.value.dbNoPrimaryKey,
  database_unsupported: t.value.dbErrorUnsupported,
  database_container_not_found: t.value.dbErrorContainer,
  network_error: t.value.dbErrorNetwork,
}))

function describe(error: unknown) {
  const response = (error as { response?: { data?: { error?: string; message?: string } } })
    ?.response
  if (response?.data?.message) return response.data.message
  const code = response?.data?.error || 'network_error'
  return errorLabels.value[code] || code
}

const isMongo = computed(() => connection.value?.engine === 'mongodb')
const filteredTables = computed(() => {
  const query = tableFilter.value.trim().toLowerCase()
  const tables = overview.value?.tables ?? []
  return query ? tables.filter((item) => item.name.toLowerCase().includes(query)) : tables
})
const tableNames = computed(() => overview.value?.tables.map((item) => item.name) ?? [])
const currentColumns = computed(() =>
  structureTable.value === table.value ? (structure.value?.columns.map((c) => c.name) ?? []) : [],
)
const initialSql = computed(() => {
  if (!connection.value) return ''
  if (isMongo.value)
    return table.value ? JSON.stringify({ find: table.value, filter: {}, limit: 50 }, null, 2) : ''
  if (!table.value) return ''
  const quote = connection.value.engine === 'mysql' ? '`' : '"'
  return `SELECT * FROM ${quote}${table.value}${quote} LIMIT 100;`
})

function persistSession() {
  try {
    // Only Docker-container sessions are restorable: they carry no password.
    if (connection.value?.container) {
      sessionStorage.setItem(
        sessionKey,
        JSON.stringify({ connection: { ...connection.value, password: '' }, table: table.value }),
      )
    } else {
      sessionStorage.removeItem(sessionKey)
    }
  } catch {
    /* storage unavailable */
  }
}

async function connect(next: Connection, restoreTable = '') {
  connecting.value = true
  connectError.value = ''
  try {
    const data = await databaseClient.overview(next)
    connection.value = { ...next, schema: data.schema || next.schema }
    overview.value = data
    saveRecent(connection.value)
    table.value = ''
    structure.value = undefined
    sqlOnly.value = false
    if (restoreTable && data.tables.some((item) => item.name === restoreTable)) {
      openTable(restoreTable)
    }
    persistSession()
  } catch (error) {
    connectError.value = describe(error)
  } finally {
    connecting.value = false
  }
}

async function refreshOverview() {
  if (!connection.value) return
  overviewLoading.value = true
  try {
    overview.value = await databaseClient.overview(connection.value)
  } catch (error) {
    connectError.value = describe(error)
  } finally {
    overviewLoading.value = false
  }
}

async function switchDatabase(database: string) {
  if (!connection.value || database === connection.value.database) return
  const previous = connection.value
  overviewLoading.value = true
  try {
    const next = { ...previous, database, schema: '' }
    const data = await databaseClient.overview(next)
    connection.value = { ...next, schema: data.schema }
    overview.value = data
    table.value = ''
    sqlOnly.value = false
    persistSession()
  } catch (error) {
    connectError.value = describe(error)
  } finally {
    overviewLoading.value = false
  }
}

async function switchSchema(schema: string) {
  if (!connection.value) return
  connection.value = { ...connection.value, schema }
  table.value = ''
  sqlOnly.value = false
  await refreshOverview()
  persistSession()
}

async function loadStructure() {
  if (!connection.value || !table.value) return
  const name = table.value
  structureLoading.value = true
  structureError.value = ''
  try {
    const data = await databaseClient.structure(connection.value, name)
    if (table.value !== name) return
    structure.value = data
    structureTable.value = name
  } catch (error) {
    if (table.value === name) structureError.value = describe(error)
  } finally {
    structureLoading.value = false
  }
}

function openTable(name: string) {
  table.value = name
  sqlOnly.value = false
  if (view.value === 'sql') view.value = 'data'
  structure.value = undefined
  structureTable.value = ''
  loadStructure()
  persistSession()
}

function openSql() {
  table.value = ''
  sqlOnly.value = true
  persistSession()
}

function showOverview() {
  table.value = ''
  sqlOnly.value = false
  persistSession()
}

function disconnect() {
  connection.value = undefined
  overview.value = undefined
  table.value = ''
  structure.value = undefined
  connectError.value = ''
  try {
    sessionStorage.removeItem(sessionKey)
  } catch {
    /* ignore */
  }
}

onMounted(() => {
  try {
    const saved = JSON.parse(sessionStorage.getItem(sessionKey) || 'null')
    if (saved?.connection?.container) connect(saved.connection, saved.table || '')
  } catch {
    /* ignore broken session */
  }
})
onBeforeUnmount(() => {
  if (connection.value) connection.value.password = ''
})

function onRowClick(event: { data: { name: string } }) {
  openTable(event.data.name)
}

const tableIcon = (type: string) =>
  type === 'view' || type === 'materialized view' ? 'pi pi-eye' : 'pi pi-table'
</script>

<template>
  <section class="space-y-4">
    <header class="page-header !mb-0">
      <div class="min-w-0">
        <h1 class="page-title">{{ t.databases }}</h1>
        <p class="page-subtitle">{{ t.databaseHint }}</p>
      </div>
      <div v-if="connection" class="flex items-center gap-2">
        <Tag :value="t.databaseConnected" severity="success" icon="pi pi-circle-fill" />
        <Button
          :label="t.disconnect"
          icon="pi pi-sign-out"
          size="small"
          severity="secondary"
          outlined
          @click="disconnect"
        />
      </div>
    </header>

    <Message v-if="connectError" severity="error" :closable="true" @close="connectError = ''">
      <span class="whitespace-pre-wrap">{{ connectError }}</span>
    </Message>

    <DbConnect v-if="!connection" :loading="connecting" @connect="connect" />

    <div v-else class="db-workspace">
      <aside class="db-side panel-card">
        <div class="db-side__head">
          <div class="flex items-center gap-2">
            <span class="db-badge"><i class="pi pi-database" /></span>
            <div class="min-w-0">
              <div class="truncate text-sm font-semibold">
                {{ engineLabels[connection.engine] }}
              </div>
              <div v-tooltip.bottom="overview?.version" class="muted truncate font-mono text-xs">
                {{ connection.container ? '' : (connection.username || '—') + '@'
                }}{{ connection.host }}:{{ connection.port }}
              </div>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <label for="db-switch" class="db-side__label">{{ t.dbDatabase }}</label>
            <Select
              :model-value="connection.database"
              input-id="db-switch"
              :options="overview?.databases.length ? overview.databases : [connection.database]"
              filter
              size="small"
              :loading="overviewLoading"
              @update:model-value="switchDatabase"
            />
          </div>
          <div v-if="overview?.schemas.length" class="flex flex-col gap-1.5">
            <label for="db-schema" class="db-side__label">{{ t.dbSchema }}</label>
            <Select
              :model-value="connection.schema"
              input-id="db-schema"
              :options="overview.schemas"
              size="small"
              @update:model-value="switchSchema"
            />
          </div>
          <div class="grid grid-cols-2 gap-1.5">
            <Button
              :label="isMongo ? t.dbMongoCommand : 'SQL'"
              icon="pi pi-code"
              size="small"
              :severity="sqlOnly ? undefined : 'secondary'"
              :outlined="!sqlOnly"
              @click="openSql"
            />
            <Button
              :label="t.dbTables"
              icon="pi pi-list"
              size="small"
              :severity="!table && !sqlOnly ? undefined : 'secondary'"
              :outlined="!!table || sqlOnly"
              @click="showOverview"
            />
          </div>
          <IconField>
            <InputIcon class="pi pi-search" />
            <InputText
              v-model="tableFilter"
              :placeholder="t.dbFilterTables"
              size="small"
              class="w-full"
              :aria-label="t.dbFilterTables"
            />
          </IconField>
        </div>
        <nav class="db-side__list" :aria-label="t.dbTables">
          <Button
            v-for="item in filteredTables"
            :key="item.name"
            text
            size="small"
            :severity="item.name === table ? undefined : 'secondary'"
            :class="['db-table-link', { 'db-table-link--active': item.name === table }]"
            @click="openTable(item.name)"
          >
            <i :class="tableIcon(item.type)" />
            <span class="min-w-0 flex-1 truncate text-left font-mono text-xs">{{ item.name }}</span>
            <span v-if="item.rows" class="db-table-link__count">{{
              item.rows.toLocaleString()
            }}</span>
          </Button>
          <p v-if="overview && !filteredTables.length" class="muted px-2 py-3 text-xs">
            {{ overview.tables.length ? '—' : t.dbEmptyDatabase }}
          </p>
        </nav>
      </aside>

      <div class="db-main min-w-0">
        <nav class="db-crumbs">
          <i class="pi pi-database" />
          <span>{{ connection.database }}</span>
          <template v-if="connection.schema">
            <i class="pi pi-angle-right" /><span>{{ connection.schema }}</span>
          </template>
          <template v-if="table">
            <i class="pi pi-angle-right" /><b>{{ table }}</b>
          </template>
          <template v-else-if="sqlOnly">
            <i class="pi pi-angle-right" /><b>{{ isMongo ? t.dbMongoCommand : t.dbSqlCommand }}</b>
          </template>
        </nav>

        <DbSql
          v-if="sqlOnly"
          :connection="connection"
          :tables="tableNames"
          :columns="[]"
          :initial="initialSql"
          :error-labels="errorLabels"
          @changed="refreshOverview"
        />

        <div v-else-if="!table" class="space-y-3">
          <div class="flex items-center gap-2">
            <h2 class="section-title">{{ t.dbOverviewTitle }}</h2>
            <span class="muted text-sm">({{ overview?.tables.length ?? 0 }})</span>
            <span class="flex-1" />
            <Button
              v-tooltip.left="t.refresh"
              icon="pi pi-refresh"
              size="small"
              text
              rounded
              severity="secondary"
              :aria-label="t.refresh"
              :loading="overviewLoading"
              @click="refreshOverview"
            />
          </div>
          <DataTable
            :value="filteredTables"
            :loading="overviewLoading"
            size="small"
            striped-rows
            scrollable
            scroll-height="65vh"
            sort-field="name"
            :sort-order="1"
            data-key="name"
            class="cursor-pointer"
            @row-click="onRowClick"
          >
            <template #empty>{{ t.dbEmptyDatabase }}</template>
            <Column field="name" :header="t.name" sortable>
              <template #body="{ data }">
                <span class="flex items-center gap-2">
                  <i :class="[tableIcon(data.type), 'muted']" />
                  <span class="font-mono text-sm text-[var(--p-primary-color)]">{{
                    data.name
                  }}</span>
                </span>
              </template>
            </Column>
            <Column field="type" :header="t.dbType" sortable class="w-36">
              <template #body="{ data }">
                <Tag :value="data.type" severity="secondary" />
              </template>
            </Column>
            <Column field="rows" :header="t.dbRowsEstimate" sortable class="w-32 text-right">
              <template #body="{ data }">
                <span class="tabular-nums">{{ data.rows.toLocaleString() }}</span>
              </template>
            </Column>
            <Column v-if="!isMongo" field="size" :header="t.dbSize" sortable class="w-28">
              <template #body="{ data }">{{ formatBytes(data.size) }}</template>
            </Column>
            <Column v-if="!isMongo" field="comment" :header="t.dbComment">
              <template #body="{ data }">
                <span class="muted text-xs">{{ data.comment }}</span>
              </template>
            </Column>
          </DataTable>
        </div>

        <template v-else>
          <Message v-if="structureError" severity="error" :closable="false">
            <span class="whitespace-pre-wrap font-mono text-xs">{{ structureError }}</span>
          </Message>
          <Skeleton v-if="structureLoading && !structure" height="18rem" />
          <Tabs v-else-if="structure && structureTable === table" v-model:value="view" lazy>
            <TabList>
              <Tab value="data"><i class="pi pi-table mr-2" />{{ t.dbData }}</Tab>
              <Tab value="structure"><i class="pi pi-sitemap mr-2" />{{ t.dbStructure }}</Tab>
              <Tab value="sql"
                ><i class="pi pi-code mr-2" />{{ isMongo ? t.dbMongoCommand : 'SQL' }}</Tab
              >
            </TabList>
            <TabPanels>
              <TabPanel value="data">
                <DbData
                  :connection="connection"
                  :table="table"
                  :structure="structure"
                  :error-labels="errorLabels"
                />
              </TabPanel>
              <TabPanel value="structure">
                <DbStructure
                  :structure="structure"
                  :engine="connection.engine"
                  :loading="structureLoading"
                  @open="openTable"
                />
              </TabPanel>
              <TabPanel value="sql">
                <DbSql
                  :connection="connection"
                  :tables="tableNames"
                  :columns="currentColumns"
                  :initial="initialSql"
                  :error-labels="errorLabels"
                  @changed="refreshOverview"
                />
              </TabPanel>
            </TabPanels>
          </Tabs>
        </template>
      </div>
    </div>
  </section>
</template>

<style scoped>
.db-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
@media (min-width: 1024px) {
  .db-workspace {
    grid-template-columns: 260px minmax(0, 1fr);
  }
  .db-side {
    position: sticky;
    top: 0;
    max-height: calc(100vh - 120px);
  }
}
.db-side {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.db-side__head {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  border-bottom: 1px solid var(--app-border);
}
.db-side__label {
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--p-text-muted-color);
}
.db-side__list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-height: 0;
  max-height: 40vh;
  overflow-y: auto;
  padding: 6px;
}
@media (min-width: 1024px) {
  .db-side__list {
    max-height: none;
    flex: 1;
  }
}
.db-badge {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  border-radius: 8px;
  color: var(--p-primary-color);
  background: color-mix(in srgb, var(--p-primary-500) 14%, transparent);
}
.db-table-link {
  justify-content: flex-start;
  gap: 8px;
  padding: 5px 8px;
  font-weight: 400;
}
.db-table-link--active {
  background: color-mix(in srgb, var(--p-primary-500) 13%, transparent);
}
.db-table-link__count {
  font-size: 0.68rem;
  font-variant-numeric: tabular-nums;
  color: var(--p-text-muted-color);
}
.db-crumbs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.85rem;
  color: var(--p-text-muted-color);
}
.db-crumbs b {
  color: var(--p-text-color);
}
.db-crumbs .pi {
  font-size: 0.75rem;
}
</style>
