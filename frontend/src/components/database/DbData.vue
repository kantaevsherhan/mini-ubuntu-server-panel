<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable, { type DataTablePageEvent, type DataTableSortEvent } from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import DbCell from './DbCell.vue'
import DbRowDialog from './DbRowDialog.vue'
import {
  databaseClient,
  downloadText,
  filterOperators,
  toCSV,
  type Cell,
  type Connection,
  type Filter,
  type ResultSet,
  type Structure,
} from '../../services/databaseClient'
import { useI18n } from '../../services/i18n'

const props = defineProps<{
  connection: Connection
  table: string
  structure: Structure
  errorLabels: Record<string, string>
}>()
const { t } = useI18n()
const confirm = useConfirm()
const toast = useToast()

interface RowItem {
  __index: number
  [column: string]: Cell
}

const result = ref<ResultSet>()
const loading = ref(false)
const error = ref('')
const offset = ref(0)
const limit = ref(Number(localStorage.getItem('database-page-size')) || 50)
const orderBy = ref('')
const desc = ref(false)
const filters = ref<Filter[]>([])
const mongoFilter = ref('')
const selection = ref<RowItem[]>([])
const isMongo = computed(() => props.connection.engine === 'mongodb')
const editable = computed(
  () =>
    (props.structure.type === 'table' || props.structure.type === 'collection') &&
    (isMongo.value || props.structure.columns.some((column) => column.primary)),
)
const readOnlyReason = computed(() => {
  if (props.structure.type !== 'table' && props.structure.type !== 'collection')
    return t.value.dbReadOnlyView
  if (!editable.value) return t.value.dbNoPrimaryKey
  return ''
})
const columnNames = computed(() => props.structure.columns.map((column) => column.name))
const limitOptions = [25, 50, 100, 200, 500]
const operatorOptions = computed(() =>
  filterOperators.filter((op) => op !== 'ILIKE' || props.connection.engine === 'postgres'),
)

const rows = computed<RowItem[]>(() => {
  const set = result.value
  if (!set) return []
  return set.rows.map((values, index) => {
    const item: RowItem = { __index: index }
    set.columns.forEach((column, i) => (item[column] = values[i]))
    return item
  })
})

let generation = 0
async function load() {
  const current = ++generation
  loading.value = true
  error.value = ''
  try {
    const data = await databaseClient.select(props.connection, {
      table: props.table,
      offset: offset.value,
      limit: limit.value,
      order_by: orderBy.value || undefined,
      desc: desc.value,
      filters: isMongo.value ? [] : filters.value.filter((f) => f.column),
      mongo_filter: isMongo.value ? mongoFilter.value : undefined,
    })
    if (current !== generation) return
    result.value = data
    selection.value = []
  } catch (e) {
    if (current !== generation) return
    error.value = errorLabelsText(e)
  } finally {
    if (current === generation) loading.value = false
  }
}

function errorLabelsText(e: unknown) {
  const response = (e as { response?: { data?: { error?: string; message?: string } } })?.response
  if (response?.data?.message) return response.data.message
  const code = response?.data?.error || 'network_error'
  return props.errorLabels[code] || code
}

watch(
  () => props.table,
  () => {
    offset.value = 0
    orderBy.value = ''
    desc.value = false
    filters.value = []
    mongoFilter.value = ''
    result.value = undefined
    load()
  },
  { immediate: true },
)
watch(limit, (value) => {
  try {
    localStorage.setItem('database-page-size', String(value))
  } catch {
    /* ignore */
  }
})

function addFilter() {
  filters.value.push({ column: columnNames.value[0] || '', op: '=', value: '' })
}

function apply() {
  offset.value = 0
  load()
}

function resetFilters() {
  filters.value = []
  mongoFilter.value = ''
  apply()
}

function onPage(event: DataTablePageEvent) {
  offset.value = event.first
  limit.value = event.rows
  load()
}

function onSort(event: DataTableSortEvent) {
  orderBy.value = typeof event.sortField === 'string' ? event.sortField : ''
  desc.value = event.sortOrder === -1
  offset.value = 0
  load()
}

function keyOf(row: RowItem) {
  const keys = result.value?.primary_key.length
    ? result.value.primary_key
    : props.structure.columns.filter((c) => c.primary).map((c) => c.name)
  return Object.fromEntries(keys.map((key) => [key, row[key]]))
}

// Row dialog
const dialogVisible = ref(false)
const dialogMode = ref<'insert' | 'update'>('insert')
const dialogRow = ref<Record<string, Cell>>()
const dialogDocument = ref('')
const dialogKey = ref<Record<string, Cell>>()
const saving = ref(false)
const dialogError = ref('')

function openInsert(template?: RowItem) {
  dialogMode.value = 'insert'
  dialogRow.value = template ? { ...template } : undefined
  dialogDocument.value = template ? (result.value?.documents?.[template.__index] ?? '') : ''
  dialogKey.value = undefined
  dialogError.value = ''
  dialogVisible.value = true
}

function openEdit(row: RowItem) {
  dialogMode.value = 'update'
  dialogRow.value = { ...row }
  dialogDocument.value = result.value?.documents?.[row.__index] ?? ''
  dialogKey.value = keyOf(row)
  dialogError.value = ''
  dialogVisible.value = true
}

async function save(payload: { values: Record<string, Cell>; document: string }) {
  if (dialogMode.value === 'update' && !isMongo.value && !Object.keys(payload.values).length) {
    dialogVisible.value = false
    return
  }
  saving.value = true
  dialogError.value = ''
  try {
    const { affected } = await databaseClient.rows(props.connection, {
      table: props.table,
      action: dialogMode.value,
      keys: dialogKey.value ? [dialogKey.value] : undefined,
      values: isMongo.value ? undefined : payload.values,
      document: isMongo.value ? payload.document : undefined,
    })
    dialogVisible.value = false
    toast.add({
      severity: 'success',
      summary: t.value.dbSaved.replace('{n}', String(affected)),
      life: 2500,
    })
    load()
  } catch (e) {
    dialogError.value = errorLabelsText(e)
  } finally {
    saving.value = false
  }
}

function remove(items: RowItem[]) {
  if (!items.length) return
  confirm.require({
    header: t.value.delete,
    message:
      items.length === 1
        ? t.value.dbDeleteRowConfirm
        : t.value.dbDeleteConfirm.replace('{n}', String(items.length)),
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: t.value.delete, severity: 'danger', icon: 'pi pi-trash' },
    rejectProps: { label: t.value.cancel, severity: 'secondary', text: true },
    accept: async () => {
      try {
        const { affected } = await databaseClient.rows(props.connection, {
          table: props.table,
          action: 'delete',
          keys: items.map(keyOf),
        })
        toast.add({
          severity: 'success',
          summary: t.value.dbDeleted.replace('{n}', String(affected)),
          life: 2500,
        })
        load()
      } catch (e) {
        toast.add({ severity: 'error', summary: errorLabelsText(e), life: 6000 })
      }
    },
  })
}

function onRowDblclick(event: { data: RowItem }) {
  if (editable.value) openEdit(event.data)
}

function exportCSV() {
  if (!result.value) return
  downloadText(`${props.table}.csv`, toCSV(result.value.columns, result.value.rows))
}

defineExpose({ reload: load, openInsert })
</script>

<template>
  <div class="space-y-3">
    <div class="db-toolbar">
      <template v-if="isMongo">
        <label for="db-mongo-filter" class="sr-only">{{ t.dbMongoFilter }}</label>
        <InputText
          id="db-mongo-filter"
          v-model="mongoFilter"
          class="min-w-0 flex-[1_1_18rem] font-mono text-sm"
          :placeholder="`${t.dbMongoFilter}: { &quot;status&quot;: &quot;active&quot; }`"
          @keydown.enter="apply"
        />
      </template>
      <div v-else class="flex min-w-0 flex-[1_1_18rem] flex-col gap-2">
        <div v-for="(filter, index) in filters" :key="index" class="db-filter">
          <Select
            v-model="filter.column"
            :options="columnNames"
            filter
            class="db-filter__column"
            :aria-label="t.dbColumn"
          />
          <Select
            v-model="filter.op"
            :options="operatorOptions"
            class="db-filter__op"
            :aria-label="t.dbType"
          />
          <InputText
            v-if="filter.op !== 'IS NULL' && filter.op !== 'IS NOT NULL'"
            v-model="filter.value"
            class="db-filter__value font-mono text-sm"
            :aria-label="t.dbValue"
            :placeholder="t.dbValue"
            @keydown.enter="apply"
          />
          <span v-else class="db-filter__value" />
          <Button
            icon="pi pi-times"
            text
            rounded
            severity="secondary"
            :aria-label="t.delete"
            @click="filters.splice(index, 1)"
          />
        </div>
        <div class="flex flex-wrap gap-2">
          <Button
            :label="t.dbAddFilter"
            icon="pi pi-filter"
            size="small"
            severity="secondary"
            outlined
            @click="addFilter"
          />
        </div>
      </div>
      <div class="flex flex-wrap items-start gap-2">
        <Button :label="t.dbApply" icon="pi pi-search" size="small" @click="apply" />
        <Button
          v-if="filters.length || mongoFilter"
          :label="t.dbReset"
          size="small"
          severity="secondary"
          text
          @click="resetFilters"
        />
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <Button
        :label="t.dbNewRow"
        icon="pi pi-plus"
        size="small"
        :disabled="!editable"
        @click="openInsert()"
      />
      <Button
        :label="`${t.dbDeleteSelected}${selection.length ? ` (${selection.length})` : ''}`"
        icon="pi pi-trash"
        size="small"
        severity="danger"
        outlined
        :disabled="!selection.length || !editable"
        @click="remove(selection)"
      />
      <Button
        label="CSV"
        icon="pi pi-download"
        size="small"
        severity="secondary"
        outlined
        :disabled="!result?.rows.length"
        @click="exportCSV"
      />
      <Button
        v-tooltip.bottom="t.refresh"
        icon="pi pi-refresh"
        size="small"
        severity="secondary"
        text
        rounded
        :aria-label="t.refresh"
        :loading="loading"
        @click="load"
      />
      <span class="flex-1" />
      <span v-if="result" class="muted text-sm">
        {{ t.dbTotal }}:
        <b class="text-[var(--p-text-color)]">{{ result.total >= 0 ? result.total : '?' }}</b> ·
        {{ result.duration_ms }} ms
      </span>
    </div>

    <Message v-if="readOnlyReason" severity="secondary" :closable="false" class="text-sm">
      {{ readOnlyReason }}
    </Message>
    <Message v-if="error" severity="error" :closable="false">
      <span class="whitespace-pre-wrap font-mono text-xs">{{ error }}</span>
    </Message>

    <DataTable
      v-model:selection="selection"
      :value="rows"
      :loading="loading"
      lazy
      paginator
      :first="offset"
      :rows="limit"
      :rows-per-page-options="limitOptions"
      :total-records="
        result && result.total >= 0
          ? result.total
          : offset + rows.length + (rows.length === limit ? limit : 0)
      "
      paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink RowsPerPageDropdown CurrentPageReport"
      current-page-report-template="{first}–{last} / {totalRecords}"
      :sort-field="orderBy || undefined"
      :sort-order="orderBy ? (desc ? -1 : 1) : undefined"
      removable-sort
      size="small"
      scrollable
      scroll-height="flex"
      class="db-grid"
      data-key="__index"
      striped-rows
      @page="onPage"
      @sort="onSort"
      @row-dblclick="onRowDblclick"
    >
      <template #empty>{{ loading ? '' : t.databaseNoRows }}</template>
      <Column v-if="editable" selection-mode="multiple" header-style="width: 2.5rem" frozen />
      <Column v-if="editable" header-style="width: 5.5rem" frozen>
        <template #body="{ data }">
          <div class="flex">
            <Button
              v-tooltip.top="t.dbEdit"
              icon="pi pi-pencil"
              text
              rounded
              size="small"
              :aria-label="t.dbEdit"
              @click="openEdit(data)"
            />
            <Button
              v-tooltip.top="t.dbClone"
              icon="pi pi-clone"
              text
              rounded
              size="small"
              severity="secondary"
              :aria-label="t.dbClone"
              @click="openInsert(data)"
            />
          </div>
        </template>
      </Column>
      <Column
        v-for="column in result?.columns || []"
        :key="column"
        :field="column"
        :sortable="true"
      >
        <template #header>
          <span class="font-mono normal-case tracking-normal">{{ column }}</span>
        </template>
        <template #body="{ data }"><DbCell :value="data[column]" /></template>
      </Column>
    </DataTable>

    <DbRowDialog
      v-model:visible="dialogVisible"
      :engine="connection.engine"
      :table="table"
      :columns="structure.columns"
      :row="dialogRow"
      :document="dialogDocument"
      :mode="dialogMode"
      :saving="saving"
      :error="dialogError"
      @save="save"
    />
  </div>
</template>

<style scoped>
.db-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--app-border);
  border-radius: 12px;
  background: var(--app-surface);
}
.db-filter {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 7.5rem auto;
  gap: 6px;
  align-items: center;
}
.db-filter__value {
  grid-column: 1 / -1;
  order: 5;
}
@media (min-width: 768px) {
  .db-filter {
    grid-template-columns: 14rem 8.5rem minmax(0, 1fr) auto;
  }
  .db-filter__value {
    grid-column: auto;
    order: 0;
  }
}
.db-grid {
  height: min(62vh, 720px);
}
.db-grid :deep(.p-datatable-thead > tr > th) {
  white-space: nowrap;
}
</style>
