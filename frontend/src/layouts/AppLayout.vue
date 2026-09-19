<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import Menu from 'primevue/menu'
import { useAuthStore } from '../stores/auth'
import { usePreferencesStore } from '../stores/preferences'
import { useI18n } from '../services/i18n'

interface NavItem {
  label: string
  icon: string
  path: string
  roles: string[]
}

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const preferences = usePreferencesStore()
const { t } = useI18n()
const userMenu = ref<InstanceType<typeof Menu>>()
const mobileMenuVisible = ref(false)
const collapsed = ref(localStorage.getItem('sidebar-collapsed') === 'true')
const desktopQuery = window.matchMedia('(min-width: 1024px)')
const desktop = ref(desktopQuery.matches)

function updateWorkspace(event: MediaQueryList | MediaQueryListEvent) {
  desktop.value = event.matches
  if (desktop.value) mobileMenuVisible.value = false
}

onMounted(() => {
  updateWorkspace(desktopQuery)
  desktopQuery.addEventListener('change', updateWorkspace)
})
onBeforeUnmount(() => desktopQuery.removeEventListener('change', updateWorkspace))
watch(collapsed, (value) => localStorage.setItem('sidebar-collapsed', String(value)))
watch(
  () => route.fullPath,
  () => (mobileMenuVisible.value = false),
)

const staff = ['admin', 'operator']
const groups = computed(() => {
  const all: Array<{ title: string; items: NavItem[] }> = [
    {
      title: t.value.navOverview,
      items: [
        { label: t.value.dashboard, icon: 'pi pi-chart-bar', path: '/', roles: [] },
        { label: t.value.system, icon: 'pi pi-server', path: '/system', roles: staff },
      ],
    },
    {
      title: t.value.navWorkloads,
      items: [
        { label: t.value.docker, icon: 'pi pi-box', path: '/docker', roles: staff },
        { label: t.value.services, icon: 'pi pi-cog', path: '/services', roles: staff },
        { label: t.value.processes, icon: 'pi pi-list', path: '/processes', roles: [] },
        { label: t.value.logs, icon: 'pi pi-align-left', path: '/logs', roles: staff },
      ],
    },
    {
      title: t.value.navTools,
      items: [
        { label: t.value.terminal, icon: 'pi pi-code', path: '/terminal', roles: staff },
        { label: t.value.files, icon: 'pi pi-folder', path: '/files', roles: staff },
      ],
    },
    {
      title: t.value.navSecurity,
      items: [
        { label: t.value.firewall, icon: 'pi pi-shield', path: '/firewall', roles: staff },
        { label: t.value.users, icon: 'pi pi-users', path: '/users', roles: staff },
        { label: t.value.audit, icon: 'pi pi-history', path: '/audit', roles: ['admin'] },
        { label: t.value.notifications, icon: 'pi pi-bell', path: '/notifications', roles: staff },
      ],
    },
    {
      title: t.value.navPanel,
      items: [{ label: t.value.settings, icon: 'pi pi-sliders-h', path: '/settings', roles: [] }],
    },
  ]
  return all
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => !item.roles.length || item.roles.includes(auth.role)),
    }))
    .filter((group) => group.items.length)
})

function isActive(path: string) {
  return path === '/' ? route.path === '/' : route.path.startsWith(path)
}

const currentTitle = computed(
  () => groups.value.flatMap((group) => group.items).find((item) => isActive(item.path))?.label,
)

function toggleTheme() {
  preferences.mode = preferences.mode === 'dark' ? 'light' : 'dark'
}

const accountItems = computed(() => [
  { label: t.value.settings, icon: 'pi pi-cog', command: () => router.push('/settings') },
  { separator: true },
  {
    label: t.value.logout,
    icon: 'pi pi-sign-out',
    command: () => {
      auth.logout().then(() => router.push('/login'))
    },
  },
])
</script>

<template>
  <div class="app-shell">
    <aside v-if="desktop" :class="['sidebar', { 'sidebar--collapsed': collapsed }]">
      <div class="sidebar__brand">
        <span class="brand-mark"><i class="pi pi-server" /></span>
        <div v-if="!collapsed" class="leading-tight">
          <div class="font-semibold">Mini Server</div>
          <div class="muted text-xs">Ubuntu panel</div>
        </div>
      </div>
      <nav class="sidebar__nav">
        <div v-for="group in groups" :key="group.title" class="nav-group">
          <div v-if="!collapsed" class="nav-group__title">{{ group.title }}</div>
          <RouterLink
            v-for="item in group.items"
            :key="item.path"
            v-tooltip.right="collapsed ? item.label : undefined"
            :to="item.path"
            :class="['nav-link', { 'nav-link--active': isActive(item.path) }]"
          >
            <i :class="item.icon" />
            <span v-if="!collapsed">{{ item.label }}</span>
          </RouterLink>
        </div>
      </nav>
      <button
        type="button"
        class="sidebar__collapse"
        :aria-label="collapsed ? t.expandSidebar : t.collapseSidebar"
        @click="collapsed = !collapsed"
      >
        <i :class="collapsed ? 'pi pi-angle-double-right' : 'pi pi-angle-double-left'" />
        <span v-if="!collapsed">{{ t.collapseSidebar }}</span>
      </button>
    </aside>

    <Drawer v-model:visible="mobileMenuVisible" position="left" header="Mini Server" class="!w-72">
      <nav class="sidebar__nav">
        <div v-for="group in groups" :key="group.title" class="nav-group">
          <div class="nav-group__title">{{ group.title }}</div>
          <RouterLink
            v-for="item in group.items"
            :key="item.path"
            :to="item.path"
            :class="['nav-link', { 'nav-link--active': isActive(item.path) }]"
          >
            <i :class="item.icon" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </div>
      </nav>
    </Drawer>

    <div class="workspace">
      <header class="topbar">
        <Button
          v-if="!desktop"
          icon="pi pi-bars"
          text
          rounded
          aria-label="Open navigation"
          @click="mobileMenuVisible = true"
        />
        <span class="topbar__title">{{ currentTitle }}</span>
        <span class="flex-1" />
        <span class="status-pill"><span class="status-pill__dot" />{{ t.online }}</span>
        <Button
          v-tooltip.bottom="preferences.mode === 'dark' ? t.light : t.dark"
          :icon="preferences.mode === 'dark' ? 'pi pi-sun' : 'pi pi-moon'"
          text
          rounded
          severity="secondary"
          :aria-label="t.theme"
          @click="toggleTheme"
        />
        <Button
          :label="auth.username || t.account"
          icon="pi pi-user"
          severity="secondary"
          text
          @click="userMenu?.toggle($event)"
        />
        <Menu ref="userMenu" :model="accountItems" popup />
      </header>
      <main class="content">
        <RouterView v-slot="{ Component }">
          <Transition name="page" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </main>
    </div>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  height: 100%;
  background: var(--app-background);
}

.sidebar {
  display: flex;
  width: 244px;
  flex-shrink: 0;
  flex-direction: column;
  border-right: 1px solid var(--app-border);
  background: var(--app-sidebar);
  transition: width 180ms ease;
}

.sidebar--collapsed {
  width: 68px;
}

.sidebar__brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 60px;
  padding: 0 16px;
  border-bottom: 1px solid var(--app-border);
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: 10px;
  color: var(--p-primary-contrast-color);
  background: linear-gradient(135deg, var(--p-primary-400), var(--p-primary-700));
  box-shadow: 0 4px 14px color-mix(in srgb, var(--p-primary-500) 35%, transparent);
}

.sidebar__nav {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
}

.nav-group + .nav-group {
  margin-top: 14px;
}

.nav-group__title {
  padding: 0 10px 6px;
  font-size: 0.68rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--p-text-muted-color);
}

.nav-link {
  display: flex;
  align-items: center;
  gap: 11px;
  margin: 1px 0;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 0.9rem;
  color: var(--p-text-muted-color);
  text-decoration: none;
  transition:
    background-color 120ms ease,
    color 120ms ease;
}

.sidebar--collapsed .nav-link {
  justify-content: center;
  padding: 10px 0;
}

.nav-link:hover {
  color: var(--p-text-color);
  background: var(--app-hover);
}

.nav-link--active {
  color: var(--p-primary-color);
  background: color-mix(in srgb, var(--p-primary-500) 13%, transparent);
  font-weight: 500;
}

.nav-link .pi {
  width: 18px;
  text-align: center;
}

.sidebar__collapse {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 8px 10px 12px;
  padding: 8px 12px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--p-text-muted-color);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
}

.sidebar--collapsed .sidebar__collapse {
  justify-content: center;
}

.sidebar__collapse:hover {
  background: var(--app-hover);
  color: var(--p-text-color);
}

.workspace {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.topbar {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 60px;
  flex-shrink: 0;
  padding: 0 16px 0 24px;
  border-bottom: 1px solid var(--app-border);
  background: color-mix(in srgb, var(--app-background) 85%, transparent);
  backdrop-filter: blur(8px);
}

.topbar__title {
  font-weight: 600;
}

.status-pill {
  display: none;
  align-items: center;
  gap: 7px;
  padding: 4px 11px;
  border: 1px solid color-mix(in srgb, #22c55e 35%, transparent);
  border-radius: 999px;
  font-size: 0.78rem;
  color: #22c55e;
  background: color-mix(in srgb, #22c55e 10%, transparent);
}

.status-pill__dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 8px currentColor;
}

@media (min-width: 640px) {
  .status-pill {
    display: inline-flex;
  }
}

.content {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 24px;
}

@media (max-width: 640px) {
  .content {
    padding: 14px;
  }
  .topbar {
    padding: 0 10px;
  }
}

.page-enter-active,
.page-leave-active {
  transition:
    opacity 120ms ease,
    transform 120ms ease;
}

.page-enter-from {
  opacity: 0;
  transform: translateY(4px);
}

.page-leave-to {
  opacity: 0;
}
</style>
