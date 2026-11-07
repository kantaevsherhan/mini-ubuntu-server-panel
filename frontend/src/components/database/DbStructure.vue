<script setup lang="ts">
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'
import type { Engine, Structure } from '../../services/databaseClient'
import { useI18n } from '../../services/i18n'

defineProps<{ structure: Structure; engine: Engine; loading: boolean }>()
const emit = defineEmits<{ open: [string] }>()
const { t } = useI18n()
const toast = useToast()

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.add({ severity: 'success', summary: t.value.dbCopied, life: 1500 })
  } catch {
    /* clipboard unavailable (insecure context): text stays selectable */
  }
}
</script>

<template>
  <div class="space-y-5">
    <DataTable
      :value="structure.columns"
      :loading="loading"
      size="small"
      striped-rows
      scrollable
      data-key="name"
    >
      <Column :header="t.dbColumn">
        <template #body="{ data }">
          <div class="flex items-center gap-2">
            <span class="font-mono text-sm font-medium">{{ data.name }}</span>
            <Tag v-if="data.primary" value="PK" severity="warn" class="!px-1.5 !py-0 !text-xs" />
            <Tag
              v-if="data.auto_increment"
              value="AI"
              severity="info"
              class="!px-1.5 !py-0 !text-xs"
            />
          </div>
        </template>
      </Column>
      <Column :header="t.dbType">
        <template #body="{ data }">
          <span class="font-mono text-xs text-[var(--p-primary-color)]">{{ data.type }}</span>
        </template>
      </Column>
      <Column header="NULL" class="w-20">
        <template #body="{ data }">
          <i v-if="data.nullable" class="pi pi-check muted" :aria-label="'NULL'" />
        </template>
      </Column>
      <Column v-if="engine !== 'mongodb'" :header="t.dbDefault">
        <template #body="{ data }">
          <span class="font-mono text-xs break-all">{{ data.default }}</span>
        </template>
      </Column>
      <Column v-if="engine !== 'mongodb'" :header="t.dbComment" field="comment" />
    </DataTable>

    <div v-if="structure.indexes.length" class="space-y-2">
      <h3 class="section-title">{{ t.dbIndexes }}</h3>
      <DataTable :value="structure.indexes" size="small" scrollable data-key="name">
        <Column :header="t.name">
          <template #body="{ data }">
            <span class="font-mono text-xs">{{ data.name }}</span>
          </template>
        </Column>
        <Column :header="t.dbType" class="w-32">
          <template #body="{ data }">
            <Tag
              :value="data.primary ? 'PRIMARY' : data.unique ? 'UNIQUE' : 'INDEX'"
              :severity="data.primary ? 'warn' : data.unique ? 'info' : 'secondary'"
            />
          </template>
        </Column>
        <Column :header="t.dbColumn">
          <template #body="{ data }">
            <span class="font-mono text-xs">{{ data.columns }}</span>
          </template>
        </Column>
      </DataTable>
    </div>

    <div v-if="structure.foreign_keys.length" class="space-y-2">
      <h3 class="section-title">{{ t.dbForeignKeys }}</h3>
      <DataTable :value="structure.foreign_keys" size="small" scrollable data-key="name">
        <Column :header="t.dbColumn">
          <template #body="{ data }">
            <span class="font-mono text-xs">{{ data.columns }}</span>
          </template>
        </Column>
        <Column :header="t.dbReferences">
          <template #body="{ data }">
            <Button
              :label="`${data.ref_table} (${data.ref_columns})`"
              link
              size="small"
              class="!p-0 font-mono"
              @click="emit('open', data.ref_table)"
            />
          </template>
        </Column>
        <Column :header="t.name">
          <template #body="{ data }">
            <span class="muted font-mono text-xs">{{ data.name }}</span>
          </template>
        </Column>
      </DataTable>
    </div>

    <div v-if="structure.ddl" class="space-y-2">
      <div class="flex items-center gap-2">
        <h3 class="section-title">DDL</h3>
        <span class="flex-1" />
        <Button
          :label="t.dbCopy"
          icon="pi pi-copy"
          size="small"
          text
          severity="secondary"
          @click="copy(structure.ddl)"
        />
      </div>
      <pre class="db-code">{{ structure.ddl }}</pre>
    </div>
  </div>
</template>

<style scoped>
.db-code {
  margin: 0;
  max-height: 28rem;
  overflow: auto;
  padding: 14px 16px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-background);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.78rem;
  line-height: 1.55;
  white-space: pre;
}
</style>
