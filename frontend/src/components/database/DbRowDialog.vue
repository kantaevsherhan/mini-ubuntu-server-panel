<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { editText, type Cell, type ColumnInfo, type Engine } from '../../services/databaseClient'
import { useI18n } from '../../services/i18n'

interface FieldState {
  value: string
  isNull: boolean
  useDefault: boolean
}

const props = defineProps<{
  engine: Engine
  table: string
  columns: ColumnInfo[]
  /** Existing row (edit) or a template (clone); undefined for an empty insert. */
  row?: Record<string, Cell>
  document?: string
  mode: 'insert' | 'update'
  saving: boolean
  error: string
}>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{
  save: [{ values: Record<string, Cell>; document: string }]
}>()
const { t } = useI18n()

const fields = reactive<Record<string, FieldState>>({})
const documentText = ref('')
const isMongo = computed(() => props.engine === 'mongodb')

function longType(type: string) {
  return /text|json|xml|char\(\d{3,}\)|varchar\(\d{3,}\)|blob|array|\[\]/i.test(type)
}

function reset() {
  for (const key of Object.keys(fields)) delete fields[key]
  for (const column of props.columns) {
    const existing = props.row?.[column.name]
    const fromRow = props.row !== undefined && column.name in props.row
    const autoValue = props.mode === 'insert' && (column.auto_increment || column.primary)
    fields[column.name] = {
      value: fromRow && !autoValue ? editText(existing) : '',
      isNull: fromRow ? existing === null : false,
      // New rows skip columns with server defaults unless the user types a value.
      useDefault:
        props.mode === 'insert' &&
        (!fromRow || autoValue) &&
        (column.auto_increment || column.default !== '' || (column.nullable && !fromRow)),
    }
  }
  if (isMongo.value) {
    let text = props.document || '{\n  \n}'
    if (props.mode === 'insert' && props.document) {
      try {
        const parsed = JSON.parse(props.document)
        delete parsed._id
        text = JSON.stringify(parsed, null, 2)
      } catch {
        /* keep the original text */
      }
    }
    documentText.value = text
  }
}

watch(visible, (open) => open && reset(), { immediate: true })

function onInput(name: string) {
  fields[name].useDefault = false
  fields[name].isNull = false
}

function save() {
  const values: Record<string, Cell> = {}
  for (const column of props.columns) {
    const field = fields[column.name]
    if (!field || field.useDefault) continue
    if (props.mode === 'update' && props.row) {
      // Only changed columns are sent on update.
      const original = props.row[column.name]
      const unchanged = field.isNull
        ? original === null
        : original !== null && editText(original) === field.value
      if (unchanged) continue
    }
    values[column.name] = field.isNull ? null : field.value
  }
  emit('save', { values, document: documentText.value })
}

const header = computed(
  () => `${props.mode === 'insert' ? t.value.dbNewRow : t.value.dbEditRow}: ${props.table}`,
)
</script>

<template>
  <Dialog
    v-model:visible="visible"
    modal
    maximizable
    :header="header"
    class="w-[min(760px,96vw)]"
    :draggable="false"
  >
    <form id="db-row-form" class="space-y-3" @submit.prevent="save">
      <template v-if="isMongo">
        <label for="db-document" class="text-sm font-medium">{{ t.dbDocument }}</label>
        <Textarea
          id="db-document"
          v-model="documentText"
          rows="16"
          class="w-full font-mono text-sm"
          spellcheck="false"
        />
      </template>
      <template v-else>
        <div v-for="column in columns" :key="column.name" class="db-field">
          <label :for="`db-field-${column.name}`" class="db-field__label">
            <span class="font-mono text-sm font-medium break-all">{{ column.name }}</span>
            <span class="muted font-mono text-xs break-all">{{ column.type }}</span>
            <span class="flex flex-wrap gap-1">
              <Tag
                v-if="column.primary"
                value="PK"
                severity="warn"
                class="!px-1.5 !py-0 !text-xs"
              />
              <Tag
                v-if="column.auto_increment"
                value="AI"
                severity="info"
                class="!px-1.5 !py-0 !text-xs"
              />
            </span>
          </label>
          <div class="min-w-0 space-y-1.5">
            <Textarea
              v-if="longType(column.type)"
              :id="`db-field-${column.name}`"
              v-model="fields[column.name].value"
              rows="3"
              auto-resize
              class="w-full font-mono text-sm"
              :placeholder="
                fields[column.name].isNull
                  ? 'NULL'
                  : fields[column.name].useDefault
                    ? column.default || 'DEFAULT'
                    : ''
              "
              spellcheck="false"
              @input="onInput(column.name)"
            />
            <InputText
              v-else
              :id="`db-field-${column.name}`"
              v-model="fields[column.name].value"
              class="w-full font-mono text-sm"
              :placeholder="
                fields[column.name].isNull
                  ? 'NULL'
                  : fields[column.name].useDefault
                    ? column.default || 'DEFAULT'
                    : ''
              "
              autocomplete="off"
              @input="onInput(column.name)"
            />
            <div class="flex flex-wrap gap-x-4 gap-y-1 text-sm">
              <label v-if="column.nullable" class="flex items-center gap-1.5">
                <Checkbox
                  v-model="fields[column.name].isNull"
                  binary
                  @change="fields[column.name].isNull && (fields[column.name].useDefault = false)"
                />
                NULL
              </label>
              <label
                v-if="mode === 'insert' && (column.default || column.auto_increment)"
                class="flex items-center gap-1.5"
              >
                <Checkbox
                  v-model="fields[column.name].useDefault"
                  binary
                  @change="fields[column.name].useDefault && (fields[column.name].isNull = false)"
                />
                {{ t.dbDefault }}
                <span v-if="column.default" class="muted font-mono text-xs">{{
                  column.default
                }}</span>
              </label>
            </div>
          </div>
        </div>
      </template>
      <Message v-if="error" severity="error" :closable="false">
        <span class="whitespace-pre-wrap font-mono text-xs">{{ error }}</span>
      </Message>
    </form>
    <template #footer>
      <Button :label="t.cancel" severity="secondary" text @click="visible = false" />
      <Button
        type="submit"
        form="db-row-form"
        :label="t.save"
        icon="pi pi-save"
        :loading="saving"
      />
    </template>
  </Dialog>
</template>

<style scoped>
.db-field {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 6px;
  padding: 10px 0;
  border-bottom: 1px solid var(--app-border);
}
.db-field:last-of-type {
  border-bottom: 0;
}
.db-field__label {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
@media (min-width: 640px) {
  .db-field {
    grid-template-columns: 200px minmax(0, 1fr);
    gap: 16px;
  }
}
</style>
