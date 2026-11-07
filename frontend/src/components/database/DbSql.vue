<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Menu from 'primevue/menu'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import DbCell from './DbCell.vue'
import {
  databaseClient,
  downloadText,
  loadHistory,
  pushHistory,
  toCSV,
  type Connection,
  type ResultSet,
} from '../../services/databaseClient'
import { loadMonaco } from '../../services/monaco'
import { useI18n } from '../../services/i18n'
import { usePreferencesStore } from '../../stores/preferences'

type MonacoModule = Awaited<ReturnType<typeof loadMonaco>>
type Editor = ReturnType<MonacoModule['editor']['create']>
type Disposable = { dispose(): void }

const props = defineProps<{
  connection: Connection
  tables: string[]
  columns: string[]
  initial: string
  errorLabels: Record<string, string>
}>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const preferences = usePreferencesStore()

const host = ref<HTMLDivElement>()
const editor = shallowRef<Editor>()
let monaco: MonacoModule | undefined
let completion: Disposable | undefined
const running = ref(false)
const error = ref('')
const results = ref<ResultSet[]>([])
const history = ref<string[]>(loadHistory())
const historyMenu = ref<InstanceType<typeof Menu>>()
const isMongo = computed(() => props.connection.engine === 'mongodb')
const language = computed(() =>
  isMongo.value ? 'javascript' : props.connection.engine === 'mysql' ? 'mysql' : 'pgsql',
)

async function registerLanguages() {
  await Promise.all([
    import('monaco-editor/esm/vs/basic-languages/pgsql/pgsql.contribution'),
    import('monaco-editor/esm/vs/basic-languages/mysql/mysql.contribution'),
    import('monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution'),
  ])
}

function registerCompletion() {
  if (!monaco) return
  completion?.dispose()
  const m = monaco
  completion = m.languages.registerCompletionItemProvider(language.value, {
    provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position)
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      }
      const quote = (name: string) =>
        /^[a-z_][a-z0-9_]*$/.test(name)
          ? name
          : props.connection.engine === 'mysql'
            ? `\`${name}\``
            : `"${name}"`
      return {
        suggestions: [
          ...props.tables.map((name) => ({
            label: name,
            kind: m.languages.CompletionItemKind.Struct,
            insertText: quote(name),
            range,
          })),
          ...props.columns.map((name) => ({
            label: name,
            kind: m.languages.CompletionItemKind.Field,
            insertText: quote(name),
            range,
          })),
        ],
      }
    },
  })
}

onMounted(async () => {
  monaco = await loadMonaco()
  await registerLanguages()
  await nextTick()
  if (!host.value) return
  editor.value = monaco.editor.create(host.value, {
    value: props.initial,
    language: language.value,
    theme: preferences.mode === 'dark' ? 'vs-dark' : 'vs',
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 13,
    tabSize: 2,
    wordWrap: 'on',
    scrollBeyondLastLine: false,
    lineNumbersMinChars: 3,
    padding: { top: 10 },
  })
  editor.value.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => run())
  registerCompletion()
  editor.value.focus()
})

watch(
  () => preferences.mode,
  (mode) => monaco?.editor.setTheme(mode === 'dark' ? 'vs-dark' : 'vs'),
)
watch(
  () => props.initial,
  (value) => {
    if (value && editor.value) editor.value.setValue(value)
  },
)
watch(language, (value) => {
  const model = editor.value?.getModel()
  if (model && monaco) monaco.editor.setModelLanguage(model, value)
  registerCompletion()
})
onBeforeUnmount(() => {
  completion?.dispose()
  editor.value?.dispose()
})

/** Executes the selection when there is one, otherwise the whole editor. */
async function run() {
  if (!editor.value || running.value) return
  const selection = editor.value.getSelection()
  const model = editor.value.getModel()
  const selected =
    selection && model && !selection.isEmpty() ? model.getValueInRange(selection) : ''
  const sql = (selected || editor.value.getValue()).trim()
  if (!sql) return
  running.value = true
  error.value = ''
  try {
    const data = await databaseClient.query(props.connection, sql)
    results.value = data.results
    history.value = pushHistory(sql)
    if (data.results.some((r) => !r.columns.length || r.affected)) emit('changed')
  } catch (e) {
    const response = (e as { response?: { data?: { error?: string; message?: string } } })?.response
    const code = response?.data?.error || 'network_error'
    error.value = response?.data?.message || props.errorLabels[code] || code
    results.value = []
  } finally {
    running.value = false
  }
}

const historyItems = computed(() =>
  history.value.map((sql) => ({
    label: sql.length > 90 ? sql.slice(0, 90).replace(/\s+/g, ' ') + '…' : sql.replace(/\s+/g, ' '),
    command: () => editor.value?.setValue(sql),
  })),
)

function rowsOf(set: ResultSet) {
  return set.rows.map((values, index) => {
    const item: Record<string, unknown> = { __index: index }
    set.columns.forEach((column, i) => (item[`c${i}`] = values[i]))
    return item
  })
}

function exportSet(set: ResultSet, index: number) {
  downloadText(`query-${index + 1}.csv`, toCSV(set.columns, set.rows))
}
</script>

<template>
  <div class="space-y-3">
    <div class="db-editor-card">
      <div ref="host" class="db-editor" />
      <div class="db-editor-bar">
        <Button :label="t.dbRun" icon="pi pi-play" size="small" :loading="running" @click="run" />
        <Button
          :label="t.dbHistory"
          icon="pi pi-history"
          size="small"
          severity="secondary"
          text
          :disabled="!history.length"
          @click="historyMenu?.toggle($event)"
        />
        <Menu
          ref="historyMenu"
          :model="historyItems"
          popup
          class="max-w-[90vw] font-mono text-xs"
        />
        <span class="muted hidden text-xs md:inline">{{
          isMongo ? t.dbMongoRunHint : t.dbRunHint
        }}</span>
      </div>
    </div>
    <Message severity="warn" :closable="false" class="text-sm">
      <i class="pi pi-exclamation-triangle mr-2" />{{ t.dbDangerHint }}
    </Message>
    <Message v-if="error" severity="error" :closable="false">
      <span class="whitespace-pre-wrap font-mono text-xs">{{ error }}</span>
    </Message>

    <section v-for="(set, index) in results" :key="index" class="db-result">
      <header class="db-result__header">
        <code class="db-result__sql">{{ set.statement }}</code>
        <span class="flex flex-wrap items-center gap-2">
          <Tag v-if="set.error" severity="danger" value="ERROR" />
          <Tag
            v-else-if="set.columns.length"
            severity="success"
            :value="t.dbRowsReturned.replace('{n}', String(set.rows.length))"
          />
          <Tag v-else severity="info" :value="t.dbAffected.replace('{n}', String(set.affected))" />
          <Tag
            v-if="set.truncated"
            severity="warn"
            :value="t.dbTruncated.replace('{n}', String(set.rows.length))"
          />
          <span class="muted text-xs">{{ set.duration_ms }} ms</span>
          <Button
            v-if="set.rows.length"
            v-tooltip.left="'CSV'"
            icon="pi pi-download"
            text
            rounded
            size="small"
            severity="secondary"
            aria-label="CSV"
            @click="exportSet(set, index)"
          />
        </span>
      </header>
      <Message v-if="set.error" severity="error" :closable="false" class="m-3">
        <span class="whitespace-pre-wrap font-mono text-xs">{{ set.error }}</span>
      </Message>
      <DataTable
        v-else-if="set.columns.length"
        :value="rowsOf(set)"
        size="small"
        scrollable
        scroll-height="24rem"
        striped-rows
        data-key="__index"
        class="db-result__table"
      >
        <Column v-for="(column, i) in set.columns" :key="i" :field="`c${i}`">
          <template #header>
            <span class="font-mono normal-case tracking-normal">{{ column }}</span>
          </template>
          <template #body="{ data }"><DbCell :value="data[`c${i}`]" /></template>
        </Column>
      </DataTable>
    </section>
  </div>
</template>

<style scoped>
.db-editor-card {
  overflow: hidden;
  border: 1px solid var(--app-border);
  border-radius: 12px;
  background: var(--app-surface);
}
.db-editor {
  height: 220px;
  resize: vertical;
  overflow: hidden;
}
.db-editor-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-top: 1px solid var(--app-border);
}
.db-result {
  overflow: hidden;
  border: 1px solid var(--app-border);
  border-radius: 12px;
  background: var(--app-surface);
}
.db-result__header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--app-border);
}
.db-result__sql {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  font-size: 0.75rem;
  color: var(--p-text-muted-color);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.db-result__table :deep(.p-datatable) {
  border: 0;
  border-radius: 0;
}
.db-result__table {
  border: 0 !important;
  border-radius: 0 !important;
}
</style>
