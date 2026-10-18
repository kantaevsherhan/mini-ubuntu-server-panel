<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Dialog from 'primevue/dialog'
import Listbox from 'primevue/listbox'
import { useNavigation } from '../composables/useNavigation'
import { useI18n } from '../services/i18n'

interface PaletteEntry {
  key: string
  label: string
  icon: string
  hint?: string
  run: () => void
}

const props = defineProps<{ actions: PaletteEntry[] }>()
const visible = defineModel<boolean>('visible', { default: false })

const router = useRouter()
const { t } = useI18n()
const { groups } = useNavigation()
const listbox = ref<InstanceType<typeof Listbox>>()
const selected = ref<PaletteEntry | null>(null)
const query = ref('')
// Listbox drops option focus whenever the filter changes; Enter then picks the first match.
const typedSinceArrow = ref(false)

const sections = computed(() => [
  ...groups.value.map((group) => ({
    title: group.title,
    entries: group.items.map<PaletteEntry>((item) => ({
      key: item.path,
      label: item.label,
      icon: item.icon,
      hint: item.path,
      run: () => router.push(item.path),
    })),
  })),
  { title: t.value.navActions, entries: props.actions },
])

const firstMatch = computed(() => {
  const needle = query.value.trim().toLocaleLowerCase()
  return sections.value
    .flatMap((section) => section.entries)
    .find((entry) =>
      [entry.label, entry.hint ?? ''].some((field) => field.toLocaleLowerCase().includes(needle)),
    )
})

function onFilter(event: { value: string }) {
  query.value = event.value
  typedSinceArrow.value = true
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') typedSinceArrow.value = false
  if (event.key === 'Enter' && typedSinceArrow.value) {
    event.preventDefault()
    event.stopPropagation()
    choose(firstMatch.value ?? null)
  }
}

function reset() {
  query.value = ''
  typedSinceArrow.value = false
  selected.value = null
}

function choose(entry: PaletteEntry | null) {
  if (!entry) return
  visible.value = false
  selected.value = null
  entry.run()
}

async function focusFilter() {
  await nextTick()
  const root = (listbox.value as { $el?: HTMLElement } | undefined)?.$el
  root?.querySelector<HTMLInputElement>('.p-listbox-filter')?.focus()
}

function onShortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    visible.value = !visible.value
  }
}

onMounted(() => window.addEventListener('keydown', onShortcut))
onBeforeUnmount(() => window.removeEventListener('keydown', onShortcut))
</script>

<template>
  <Dialog
    v-model:visible="visible"
    modal
    dismissable-mask
    :show-header="false"
    position="top"
    class="command-palette w-[min(560px,calc(100vw-24px))]"
    :pt="{ content: { class: '!p-0' } }"
    @show="focusFilter"
    @hide="reset"
  >
    <div @keydown.capture="onKeydown">
      <Listbox
        ref="listbox"
        v-model="selected"
        :options="sections"
        option-label="label"
        option-group-label="title"
        option-group-children="entries"
        data-key="key"
        filter
        :filter-fields="['label', 'hint']"
        :filter-placeholder="t.quickNavigationPlaceholder"
        :empty-filter-message="t.quickNavigationEmpty"
        auto-option-focus
        focus-on-hover
        scroll-height="min(420px, 60vh)"
        :aria-label="t.quickNavigation"
        class="!border-0 !bg-transparent !shadow-none"
        @update:model-value="choose"
        @filter="onFilter"
      >
        <template #optiongroup="{ option }">
          <span class="text-xs font-medium text-[var(--p-text-muted-color)]">{{
            option.title
          }}</span>
        </template>
        <template #option="{ option }">
          <div class="flex w-full items-center gap-3">
            <i :class="[option.icon, 'w-4 text-center text-[var(--p-text-muted-color)]']" />
            <span class="flex-1">{{ option.label }}</span>
            <span v-if="option.hint" class="font-mono text-xs text-[var(--p-text-muted-color)]">{{
              option.hint
            }}</span>
          </div>
        </template>
      </Listbox>
    </div>
    <footer class="palette-footer">
      <span><kbd class="kbd">↑</kbd><kbd class="kbd">↓</kbd></span>
      <span><kbd class="kbd">Enter</kbd></span>
      <span class="flex-1" />
      <span><kbd class="kbd">Esc</kbd></span>
    </footer>
  </Dialog>
</template>

<style scoped>
.palette-footer {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  border-top: 1px solid var(--app-border);
}

.palette-footer span {
  display: inline-flex;
  gap: 4px;
}

:deep(.p-listbox-header) {
  padding: 10px;
  border-bottom: 1px solid var(--app-border);
}

:deep(.p-listbox-list) {
  padding: 6px;
}
</style>
