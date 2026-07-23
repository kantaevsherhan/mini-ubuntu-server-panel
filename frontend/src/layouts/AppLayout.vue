<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import Menubar from 'primevue/menubar'
import Menu from 'primevue/menu'
import PanelMenu from 'primevue/panelmenu'
import Splitter from 'primevue/splitter'
import SplitterPanel from 'primevue/splitterpanel'
import Tag from 'primevue/tag'
import { useAuthStore } from '../stores/auth'
import { usePreferencesStore } from '../stores/preferences'
import { useI18n } from '../services/i18n'

const router = useRouter()
const auth = useAuthStore()
const preferences = usePreferencesStore()
const { t } = useI18n()
const userMenu = ref<InstanceType<typeof Menu>>()
const mobileMenuVisible = ref(false)

function go(path: string) {
  mobileMenuVisible.value = false
  router.push(path)
}

const navigation = computed(() => [
  { label: t.value.dashboard, icon: 'pi pi-chart-bar', command: () => go('/') },
  { label: t.value.docker, icon: 'pi pi-box', command: () => go('/docker') },
  { label: t.value.processes, icon: 'pi pi-list', command: () => go('/processes') },
  { label: t.value.services, icon: 'pi pi-cog', command: () => go('/services') },
  { label: t.value.terminal, icon: 'pi pi-desktop', command: () => go('/terminal') },
  { label: t.value.files, icon: 'pi pi-folder', command: () => go('/files') },
  { label: t.value.users, icon: 'pi pi-users', command: () => go('/users') },
  { label: t.value.firewall, icon: 'pi pi-shield', command: () => go('/firewall') },
  { label: t.value.logs, icon: 'pi pi-align-left', command: () => go('/logs') },
  { label: t.value.audit, icon: 'pi pi-history', command: () => go('/audit') },
  { label: t.value.notifications, icon: 'pi pi-bell', command: () => go('/notifications') },
  { label: t.value.settings, icon: 'pi pi-sliders-h', command: () => go('/settings') },
])

const accountItems = computed(() => [
  { label: t.value.settings, icon: 'pi pi-cog', command: () => router.push('/settings') },
  { separator: true },
  {
    label: t.value.logout,
    icon: 'pi pi-sign-out',
    command: () => {
      auth.logout()
      router.push('/login')
    },
  },
])

const sidebarPercent = computed(() => Math.min(30, Math.max(15, preferences.sidebarWidth / 12)))

function saveSidebar(event: { sizes?: number[] }) {
  const firstPanel = event.sizes?.[0]
  if (firstPanel) preferences.sidebarWidth = Math.round(firstPanel * 12)
}
</script>

<template>
  <div class="flex h-full flex-col">
    <Menubar :model="[]" class="shrink-0 rounded-none border-x-0 border-t-0">
      <template #start>
        <div class="flex items-center gap-3">
          <Button
            icon="pi pi-bars"
            text
            rounded
            class="lg:hidden"
            aria-label="Open navigation"
            @click="mobileMenuVisible = true"
          />
          <i class="pi pi-server text-xl text-primary" />
          <strong class="hidden sm:inline">Mini Ubuntu Server Panel</strong>
          <span class="muted hidden text-sm md:inline">Ubuntu 24.04</span>
        </div>
      </template>
      <template #end>
        <div class="flex items-center gap-2">
          <Tag severity="success" class="hidden sm:inline-flex">
            <i class="pi pi-circle-fill mr-2 text-[8px]" />{{ t.online }}
          </Tag>
          <Button
            label="admin"
            icon="pi pi-user"
            icon-pos="right"
            text
            @click="userMenu?.toggle($event)"
          />
          <Menu ref="userMenu" :model="accountItems" popup />
        </div>
      </template>
    </Menubar>

    <Drawer v-model:visible="mobileMenuVisible" position="left" header="Mini Ubuntu Server">
      <PanelMenu :model="navigation" class="border-0" />
    </Drawer>

    <Splitter class="hidden min-h-0 flex-1 rounded-none border-0 lg:flex" @resizeend="saveSidebar">
      <SplitterPanel :size="sidebarPercent" :min-size="15" class="overflow-auto p-2">
        <PanelMenu :model="navigation" class="border-0" />
      </SplitterPanel>
      <SplitterPanel :size="100 - sidebarPercent" :min-size="60" class="overflow-auto">
        <main class="p-5"><RouterView /></main>
      </SplitterPanel>
    </Splitter>
    <main class="min-h-0 flex-1 overflow-auto p-3 sm:p-5 lg:hidden"><RouterView /></main>
  </div>
</template>
