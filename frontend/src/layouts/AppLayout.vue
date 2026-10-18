<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import Menu from 'primevue/menu'
import CommandPalette from '../components/CommandPalette.vue'
import { useNavigation } from '../composables/useNavigation'
import { serverOnline } from '../services/connection'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'
import { usePreferencesStore } from '../stores/preferences'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const preferences = usePreferencesStore()
const { t } = useI18n()
const { groups, isActive, current } = useNavigation()
const userMenu = ref<InstanceType<typeof Menu>>()
const mobileMenuVisible = ref(false)
const paletteVisible = ref(false)
const collapsed = ref(localStorage.getItem('sidebar-collapsed') === 'true')
const desktopQuery = window.matchMedia('(min-width: 1024px)')
const desktop = ref(desktopQuery.matches)
const shortcutLabel = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘K' : 'Ctrl K'

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

const initials = computed(() => (auth.username || '?').slice(0, 2).toUpperCase())

function toggleTheme() {
  preferences.mode = preferences.mode === 'dark' ? 'light' : 'dark'
}

function logout() {
  auth.logout().then(() => router.push('/login'))
}

const accountItems = computed(() => [
  { label: t.value.settings, icon: 'pi pi-cog', command: () => router.push('/settings') },
  { separator: true },
  { label: t.value.logout, icon: 'pi pi-sign-out', command: logout },
])

const paletteActions = computed(() => [
  {
    key: 'action:theme',
    label: t.value.toggleTheme,
    icon: preferences.mode === 'dark' ? 'pi pi-sun' : 'pi pi-moon',
    run: toggleTheme,
  },
  { key: 'action:logout', label: t.value.logout, icon: 'pi pi-sign-out', run: logout },
])
</script>

<template>
  <div class="app-shell">
    <aside v-if="desktop" :class="['sidebar', { 'sidebar--collapsed': collapsed }]">
      <div class="sidebar__brand">
        <span class="brand-mark"><i class="pi pi-server" /></span>
        <div v-if="!collapsed" class="min-w-0 leading-tight">
          <div class="brand-name">Mini Server</div>
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
      <div class="sidebar__footer">
        <Button
          v-tooltip.right="collapsed ? t.expandSidebar : undefined"
          :icon="collapsed ? 'pi pi-angle-double-right' : 'pi pi-angle-double-left'"
          :label="collapsed ? undefined : t.collapseSidebar"
          :aria-label="collapsed ? t.expandSidebar : t.collapseSidebar"
          text
          severity="secondary"
          size="small"
          class="sidebar__collapse"
          @click="collapsed = !collapsed"
        />
      </div>
    </aside>

    <Drawer v-model:visible="mobileMenuVisible" position="left" class="!w-72">
      <template #header>
        <div class="flex items-center gap-2.5">
          <span class="brand-mark"><i class="pi pi-server" /></span>
          <span class="brand-name">Mini Server</span>
        </div>
      </template>
      <nav class="sidebar__nav !p-0">
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
          severity="secondary"
          :aria-label="t.openNavigation"
          @click="mobileMenuVisible = true"
        />
        <nav v-if="current" class="breadcrumb" aria-label="Breadcrumb">
          <template v-if="current.group !== current.item.label">
            <span class="breadcrumb__group">{{ current.group }}</span>
            <i class="pi pi-angle-right breadcrumb__sep" />
          </template>
          <span class="breadcrumb__page">{{ current.item.label }}</span>
        </nav>
        <span class="flex-1" />
        <Button
          severity="secondary"
          outlined
          size="small"
          class="search-trigger"
          :aria-label="t.quickNavigation"
          @click="paletteVisible = true"
        >
          <i class="pi pi-search text-xs" />
          <span class="search-trigger__label">{{ t.search }}</span>
          <kbd class="kbd search-trigger__kbd">{{ shortcutLabel }}</kbd>
        </Button>
        <span
          :class="['status-pill', serverOnline ? 'status-pill--ok' : 'status-pill--down']"
          role="status"
          :aria-label="serverOnline ? t.online : t.offline"
        >
          <i :class="serverOnline ? 'status-pill__dot' : 'pi pi-exclamation-triangle text-xs'" />
          <span class="status-pill__label">{{ serverOnline ? t.online : t.offline }}</span>
        </span>
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
          text
          severity="secondary"
          class="account-trigger"
          :aria-label="t.account"
          aria-haspopup="true"
          @click="userMenu?.toggle($event)"
        >
          <Avatar :label="initials" shape="circle" class="account-avatar" />
          <span class="account-trigger__text">
            <span class="account-trigger__name">{{ auth.username || t.account }}</span>
            <span class="account-trigger__role">{{ auth.role }}</span>
          </span>
          <i class="pi pi-angle-down text-xs" />
        </Button>
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

    <CommandPalette v-model:visible="paletteVisible" :actions="paletteActions" />
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
  width: 248px;
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
  padding: 0 17px;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  border-radius: 9px;
  font-size: 0.9rem;
  color: var(--p-primary-contrast-color);
  background: linear-gradient(140deg, var(--p-primary-400), var(--p-primary-700));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.25),
    0 4px 12px color-mix(in srgb, var(--p-primary-500) 30%, transparent);
}

.brand-name {
  font-weight: 600;
  letter-spacing: -0.01em;
}

.sidebar__nav {
  flex: 1;
  overflow-y: auto;
  padding: 6px 10px 10px;
}

.nav-group + .nav-group {
  margin-top: 16px;
}

.nav-group__title {
  padding: 0 10px 6px;
  font-size: 0.72rem;
  font-weight: 500;
  color: var(--p-text-muted-color);
  opacity: 0.8;
}

.nav-link {
  position: relative;
  display: flex;
  align-items: center;
  gap: 11px;
  margin: 1px 0;
  padding: 7px 10px;
  border-radius: 8px;
  font-size: 0.875rem;
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

.nav-link:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}

.nav-link--active {
  color: var(--p-text-color);
  background: var(--app-hover);
  font-weight: 500;
}

.nav-link--active::before {
  position: absolute;
  top: 7px;
  bottom: 7px;
  left: -10px;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: var(--p-primary-color);
  content: '';
}

.nav-link--active .pi {
  color: var(--p-primary-color);
}

.nav-link .pi {
  width: 18px;
  font-size: 0.95rem;
  text-align: center;
}

.sidebar__footer {
  padding: 10px;
  border-top: 1px solid var(--app-border);
}

.sidebar__collapse {
  width: 100%;
  justify-content: flex-start;
}

.sidebar--collapsed .sidebar__collapse {
  justify-content: center;
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
  gap: 8px;
  height: 60px;
  flex-shrink: 0;
  padding: 0 14px 0 24px;
  border-bottom: 1px solid var(--app-border);
  background: color-mix(in srgb, var(--app-background) 82%, transparent);
  backdrop-filter: saturate(1.4) blur(10px);
}

.breadcrumb {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  font-size: 0.875rem;
}

.breadcrumb__group {
  color: var(--p-text-muted-color);
}

.breadcrumb__sep {
  font-size: 0.7rem;
  color: var(--p-text-muted-color);
  opacity: 0.6;
}

.breadcrumb__page {
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.search-trigger {
  gap: 8px;
  min-width: 200px;
  justify-content: flex-start;
  border-color: var(--app-border) !important;
  color: var(--p-text-muted-color) !important;
  background: var(--app-surface) !important;
}

.search-trigger__label {
  flex: 1;
  text-align: left;
  font-weight: 400;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 28px;
  padding: 0 10px;
  border: 1px solid;
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 500;
}

.status-pill--ok {
  border-color: color-mix(in srgb, var(--app-ok) 30%, transparent);
  color: var(--app-ok);
  background: color-mix(in srgb, var(--app-ok) 9%, transparent);
}

.status-pill--down {
  border-color: color-mix(in srgb, var(--app-danger) 35%, transparent);
  color: var(--app-danger);
  background: color-mix(in srgb, var(--app-danger) 10%, transparent);
}

.status-pill__dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 22%, transparent);
}

.account-trigger {
  gap: 9px;
  padding-inline: 6px 10px !important;
}

.account-avatar {
  width: 28px !important;
  height: 28px !important;
  font-size: 0.72rem !important;
  font-weight: 600;
  color: var(--p-primary-contrast-color) !important;
  background: linear-gradient(140deg, var(--p-primary-400), var(--p-primary-700)) !important;
}

.account-trigger__text {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.15;
}

.account-trigger__name {
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--p-text-color);
}

.account-trigger__role {
  font-size: 0.7rem;
  color: var(--p-text-muted-color);
}

.content {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 28px;
  background:
    radial-gradient(1200px 380px at 70% -120px, var(--app-glow), transparent 70%) no-repeat,
    var(--app-background);
}

@media (max-width: 1279px) {
  .search-trigger {
    min-width: 0;
  }
  .search-trigger__label,
  .search-trigger__kbd,
  .account-trigger__text {
    display: none;
  }
}

@media (max-width: 640px) {
  .content {
    padding: 14px;
  }
  .topbar {
    gap: 4px;
    padding: 0 8px;
  }
  .breadcrumb__group,
  .breadcrumb__sep,
  .status-pill__label {
    display: none;
  }
  .status-pill {
    width: 28px;
    justify-content: center;
    padding: 0;
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
