<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import Breadcrumb from 'primevue/breadcrumb'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable, { type DataTableRowDoubleClickEvent } from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import FileUpload, { type FileUploadUploaderEvent } from 'primevue/fileupload'
import FloatLabel from 'primevue/floatlabel'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
import { loadMonaco } from '../services/monaco'
import { usePreferencesStore } from '../stores/preferences'

interface FileRoot {
  id: number
  name: string
  path: string
}

interface FileEntry {
  name: string
  path: string
  directory: boolean
  symlink: boolean
  size: number
  mode: string
  modified_at: string
}

interface TextFile {
  path: string
  content: string
  size: number
  modified_at: string
}

const roots = ref<FileRoot[]>([])
const selectedRoot = ref<number>()
const currentPath = ref('')
const entries = ref<FileEntry[]>([])
const search = ref('')
const loading = ref(false)
const saving = ref(false)
const createVisible = ref(false)
const editorVisible = ref(false)
const editorHost = ref<HTMLElement>()
const editorPath = ref('')
const createForm = reactive({ type: 'file', name: '' })
const toast = useToast()
const confirm = useConfirm()
const preferences = usePreferencesStore()
const { t, locale } = useI18n()
let editor:
  import('monaco-editor/esm/vs/editor/editor.api').editor.IStandaloneCodeEditor | undefined
let monaco: typeof import('monaco-editor/esm/vs/editor/editor.api') | undefined

const selectedRootInfo = computed(() => roots.value.find((root) => root.id === selectedRoot.value))
const filtered = computed(() => {
  const value = search.value.trim().toLocaleLowerCase()
  if (!value) return entries.value
  return entries.value.filter((entry) => entry.name.toLocaleLowerCase().includes(value))
})
const breadcrumbItems = computed(() => {
  const parts = currentPath.value.split('/').filter(Boolean)
  return parts.map((part, index) => ({
    label: part,
    command: () => navigate(parts.slice(0, index + 1).join('/')),
  }))
})
const breadcrumbHome = computed(() => ({
  icon: 'pi pi-folder-open',
  label: selectedRootInfo.value?.name || t.value.files,
  command: () => navigate(''),
}))
const createTypes = computed(() => [
  { label: t.value.file, value: 'file' },
  { label: t.value.folder, value: 'folder' },
])
const createValid = computed(
  () =>
    createForm.name.trim().length > 0 &&
    createForm.name !== '.' &&
    createForm.name !== '..' &&
    !/[\\/\0]/.test(createForm.name),
)

function joinPath(directory: string, name: string) {
  return [directory, name].filter(Boolean).join('/')
}

async function loadRoots() {
  roots.value = (await api.get<FileRoot[]>('/files/roots')).data
  if (roots.value.length && selectedRoot.value === undefined) selectedRoot.value = roots.value[0].id
}

async function loadEntries() {
  if (selectedRoot.value === undefined) return
  loading.value = true
  try {
    entries.value = (
      await api.get<FileEntry[]>('/files', {
        params: { root: selectedRoot.value, path: currentPath.value },
      })
    ).data
  } finally {
    loading.value = false
  }
}

async function changeRoot() {
  currentPath.value = ''
  await loadEntries()
}

async function navigate(path: string) {
  currentPath.value = path
  search.value = ''
  await loadEntries()
}

function goUp() {
  const parts = currentPath.value.split('/').filter(Boolean)
  parts.pop()
  void navigate(parts.join('/'))
}

async function activate(entry: FileEntry) {
  if (entry.symlink) return
  if (entry.directory) await navigate(entry.path)
  else await openEditor(entry.path)
}

function onRowDoubleClick(event: DataTableRowDoubleClickEvent) {
  void activate(event.data as FileEntry)
}

async function openEditor(path: string, initialContent?: string) {
  let content = initialContent
  if (content === undefined) {
    const response = await api.get<TextFile>('/files/content', {
      params: { root: selectedRoot.value, path },
    })
    content = response.data.content
  }
  editorPath.value = path
  editorVisible.value = true
  await nextTick()
  monaco = await loadMonaco()
  editor?.dispose()
  editor = monaco.editor.create(editorHost.value as HTMLElement, {
    value: content,
    language: languageFor(path),
    theme: preferences.mode === 'dark' ? 'vs-dark' : 'vs',
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 13,
    tabSize: 2,
    wordWrap: 'on',
  })
}

function languageFor(path: string) {
  const extension = path.split('.').pop()?.toLowerCase()
  const languages: Record<string, string> = {
    json: 'json',
    js: 'javascript',
    ts: 'typescript',
    css: 'css',
    html: 'html',
    md: 'markdown',
    sh: 'shell',
    go: 'go',
    yaml: 'yaml',
    yml: 'yaml',
    ini: 'ini',
  }
  return languages[extension || ''] || 'plaintext'
}

async function saveEditor() {
  if (selectedRoot.value === undefined || !editor) return
  saving.value = true
  try {
    await api.put('/files/content', {
      root: selectedRoot.value,
      path: editorPath.value,
      content: editor.getValue(),
    })
    toast.add({ severity: 'success', summary: t.value.fileSaved, life: 3000 })
    await loadEntries()
  } finally {
    saving.value = false
  }
}

function closeEditor() {
  editor?.dispose()
  editor = undefined
}

function openCreate() {
  Object.assign(createForm, { type: 'file', name: '' })
  createVisible.value = true
}

async function createEntry() {
  if (!createValid.value || selectedRoot.value === undefined) return
  const target = joinPath(currentPath.value, createForm.name.trim())
  createVisible.value = false
  if (createForm.type === 'folder') {
    await api.post('/files/directories', { root: selectedRoot.value, path: target })
    toast.add({ severity: 'success', summary: t.value.folderCreated, life: 3000 })
    await loadEntries()
  } else {
    await openEditor(target, '')
  }
}

async function uploadFile(event: FileUploadUploaderEvent) {
  if (selectedRoot.value === undefined) return
  const selected = Array.isArray(event.files) ? event.files[0] : event.files
  if (!selected) return
  const body = new FormData()
  body.append('root', String(selectedRoot.value))
  body.append('path', currentPath.value)
  body.append('file', selected)
  await api.post('/files/upload', body)
  toast.add({ severity: 'success', summary: t.value.fileUploaded, life: 3000 })
  await loadEntries()
}

function removeEntry(entry: FileEntry) {
  confirm.require({
    header: t.value.deleteFileEntry,
    message: `${t.value.deleteFileConfirm}: ${entry.name}`,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: t.value.cancel,
    acceptLabel: t.value.delete,
    acceptClass: 'p-button-danger',
    accept: async () => {
      await api.delete('/files', { params: { root: selectedRoot.value, path: entry.path } })
      toast.add({ severity: 'success', summary: t.value.deleted, life: 3000 })
      await loadEntries()
    },
  })
}

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[index]}`
}

watch(
  () => preferences.mode,
  (mode) => monaco?.editor.setTheme(mode === 'dark' ? 'vs-dark' : 'vs'),
)

onMounted(async () => {
  await loadRoots()
  await loadEntries()
})
onBeforeUnmount(closeEditor)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
      <div>
        <h1 class="text-2xl font-semibold">{{ t.files }}</h1>
        <p class="muted mt-1 text-sm">{{ t.filesHint }}</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button
          icon="pi pi-arrow-up"
          severity="secondary"
          :disabled="!currentPath"
          :aria-label="t.parentFolder"
          @click="goUp"
        />
        <Button :label="t.newEntry" icon="pi pi-plus" @click="openCreate" />
        <FileUpload
          mode="basic"
          name="file"
          custom-upload
          auto
          :max-file-size="2097152"
          :choose-label="t.uploadFile"
          choose-icon="pi pi-upload"
          @uploader="uploadFile"
        />
        <Button
          icon="pi pi-refresh"
          severity="secondary"
          :loading="loading"
          :aria-label="t.refresh"
          @click="loadEntries"
        />
      </div>
    </div>

    <div class="grid grid-cols-1 gap-3 lg:grid-cols-[minmax(18rem,24rem)_1fr]">
      <Select
        v-model="selectedRoot"
        :options="roots"
        option-label="path"
        option-value="id"
        :placeholder="t.allowedDirectory"
        @change="changeRoot"
      />
      <InputText v-model="search" :placeholder="t.searchFiles" />
    </div>

    <div class="flex items-center gap-2 overflow-x-auto">
      <Breadcrumb :home="breadcrumbHome" :model="breadcrumbItems" class="min-w-max flex-1" />
      <Tag v-if="selectedRootInfo" :value="selectedRootInfo.path" severity="secondary" />
    </div>

    <DataTable
      :value="filtered"
      :loading="loading"
      scrollable
      scroll-height="calc(100vh - 18rem)"
      :virtual-scroller-options="{ itemSize: 48, delay: 100 }"
      size="small"
      striped-rows
      data-key="path"
      class="min-h-96"
      @row-dblclick="onRowDoubleClick"
    >
      <Column field="name" :header="t.name" sortable>
        <template #body="{ data }"
          ><Button
            :label="data.name"
            :icon="data.symlink ? 'pi pi-link' : data.directory ? 'pi pi-folder' : 'pi pi-file'"
            text
            :disabled="data.symlink"
            class="max-w-full"
            @click="activate(data)"
        /></template>
      </Column>
      <Column field="size" :header="t.size" sortable class="w-28">
        <template #body="{ data }">{{ data.directory ? '—' : formatSize(data.size) }}</template>
      </Column>
      <Column field="mode" :header="t.permissions" class="w-36"
        ><template #body="{ data }"
          ><span class="font-mono text-xs">{{ data.mode }}</span></template
        ></Column
      >
      <Column field="modified_at" :header="t.modified" sortable class="w-48"
        ><template #body="{ data }">{{
          formatDateTime(data.modified_at, locale)
        }}</template></Column
      >
      <Column :header="t.actions" frozen align-frozen="right" class="w-28">
        <template #body="{ data }"
          ><div class="flex gap-1">
            <Button
              v-if="!data.directory && !data.symlink"
              icon="pi pi-pencil"
              text
              :aria-label="t.edit"
              @click="openEditor(data.path)"
            /><Button
              icon="pi pi-trash"
              text
              severity="danger"
              :aria-label="t.delete"
              @click="removeEntry(data)"
            /></div
        ></template>
      </Column>
      <template #empty>{{ t.noFiles }}</template>
    </DataTable>

    <Dialog
      v-model:visible="createVisible"
      modal
      :header="t.newEntry"
      class="w-[min(30rem,calc(100vw-2rem))]"
    >
      <form class="space-y-5 pt-3" @submit.prevent="createEntry">
        <FloatLabel variant="on"
          ><Select
            v-model="createForm.type"
            input-id="entry-type"
            :options="createTypes"
            option-label="label"
            option-value="value"
            class="w-full"
          /><label for="entry-type">{{ t.type }}</label></FloatLabel
        >
        <FloatLabel variant="on"
          ><InputText
            id="entry-name"
            v-model="createForm.name"
            maxlength="255"
            class="w-full"
            :invalid="createForm.name.length > 0 && !createValid"
          /><label for="entry-name">{{ t.name }}</label></FloatLabel
        >
        <Message v-if="createForm.name.length > 0 && !createValid" severity="error" size="small">{{
          t.invalidFileName
        }}</Message>
        <div class="flex justify-end gap-2">
          <Button
            :label="t.cancel"
            severity="secondary"
            text
            type="button"
            @click="createVisible = false"
          /><Button :label="t.create" icon="pi pi-plus" type="submit" :disabled="!createValid" />
        </div>
      </form>
    </Dialog>

    <Dialog
      v-model:visible="editorVisible"
      modal
      maximizable
      :header="editorPath"
      class="w-[min(92rem,calc(100vw-2rem))]"
      @hide="closeEditor"
    >
      <div ref="editorHost" class="h-[65vh] min-h-80 overflow-hidden rounded" />
      <template #footer
        ><Button
          :label="t.cancel"
          severity="secondary"
          text
          @click="editorVisible = false" /><Button
          :label="t.save"
          icon="pi pi-save"
          :loading="saving"
          @click="saveEditor"
      /></template>
    </Dialog>
  </section>
</template>
