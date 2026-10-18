import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

export interface NavItem {
  label: string
  icon: string
  path: string
  roles: string[]
}

export interface NavGroup {
  title: string
  items: NavItem[]
}

const staff = ['admin', 'operator']

/** Role-filtered navigation shared by the sidebar, mobile drawer and command palette. */
export function useNavigation() {
  const route = useRoute()
  const auth = useAuthStore()
  const { t } = useI18n()

  const groups = computed<NavGroup[]>(() => {
    const all: NavGroup[] = [
      {
        title: t.value.navOverview,
        items: [
          { label: t.value.dashboard, icon: 'pi pi-objects-column', path: '/', roles: [] },
          { label: t.value.system, icon: 'pi pi-server', path: '/system', roles: staff },
          {
            label: t.value.packageUpdates,
            icon: 'pi pi-download',
            path: '/updates',
            roles: staff,
          },
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
          {
            label: t.value.notifications,
            icon: 'pi pi-bell',
            path: '/notifications',
            roles: staff,
          },
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

  const current = computed(() => {
    for (const group of groups.value) {
      const item = group.items.find((entry) => isActive(entry.path))
      if (item) return { group: group.title, item }
    }
    return undefined
  })

  return { groups, isActive, current }
}
