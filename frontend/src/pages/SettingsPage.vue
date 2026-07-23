<script setup lang="ts">
import Button from 'primevue/button'
import Card from 'primevue/card'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import TelegramSettings from '../components/TelegramSettings.vue'
import { usePreferencesStore } from '../stores/preferences'
import { useI18n } from '../services/i18n'
const p = usePreferencesStore(),
  { t } = useI18n()
const presets = [
    { label: 'Aura', value: 'aura' },
    { label: 'Lara', value: 'lara' },
  ],
  modes = [
    { label: 'Dark', value: 'dark' },
    { label: 'Light', value: 'light' },
  ],
  locales = [
    { label: 'Русский', value: 'ru' },
    { label: 'English', value: 'en' },
  ],
  colors = [
    { label: 'Emerald', value: 'emerald' },
    { label: 'Blue', value: 'blue' },
    { label: 'Violet', value: 'violet' },
  ]
</script>
<template>
  <section>
    <h1 class="mb-5 text-2xl font-semibold">{{ t.settings }}</h1>
    <Tabs value="interface"
      ><TabList
        ><Tab value="interface"><i class="pi pi-palette mr-2" />{{ t.appearance }}</Tab
        ><Tab value="telegram"><i class="pi pi-send mr-2" />Telegram</Tab></TabList
      ><TabPanels
        ><TabPanel value="interface"
          ><Card class="max-w-3xl"
            ><template #title>{{ t.appearance }}</template
            ><template #content
              ><div class="grid grid-cols-1 gap-5 md:grid-cols-2">
                <label
                  ><span class="muted mb-2 block text-sm">PrimeVue preset</span
                  ><Select
                    v-model="p.preset"
                    :options="presets"
                    option-label="label"
                    option-value="value"
                    class="w-full" /></label
                ><label
                  ><span class="muted mb-2 block text-sm">Color mode</span
                  ><Select
                    v-model="p.mode"
                    :options="modes"
                    option-label="label"
                    option-value="value"
                    class="w-full" /></label
                ><label
                  ><span class="muted mb-2 block text-sm">Accent color</span
                  ><Select
                    v-model="p.accent"
                    :options="colors"
                    option-label="label"
                    option-value="value"
                    class="w-full" /></label
                ><label
                  ><span class="muted mb-2 block text-sm">{{ t.language }}</span
                  ><Select
                    v-model="p.locale"
                    :options="locales"
                    option-label="label"
                    option-value="value"
                    class="w-full"
                /></label>
              </div>
              <div class="mt-5">
                <Button label="Preview" icon="pi pi-palette" /></div></template></Card></TabPanel
        ><TabPanel value="telegram"><TelegramSettings /></TabPanel></TabPanels
    ></Tabs>
  </section>
</template>
