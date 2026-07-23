import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../pages/LoginPage.vue') },
    { path: '/change-password', component: () => import('../pages/ChangePasswordPage.vue') },
    {
      path: '/',
      component: () => import('../layouts/AppLayout.vue'),
      children: [
        { path: '', component: () => import('../pages/DashboardPage.vue') },
        { path: 'processes', component: () => import('../pages/ProcessesPage.vue') },
        {
          path: 'users',
          component: () => import('../pages/UsersPage.vue'),
          meta: { roles: ['admin', 'operator'] },
        },
        { path: 'settings', component: () => import('../pages/SettingsPage.vue') },
        {
          path: ':section',
          component: () => import('../pages/PlaceholderPage.vue'),
          meta: { sectionRoles: true },
        },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const authenticated = Boolean(sessionStorage.getItem('access_token'))
  const mustChangePassword = sessionStorage.getItem('must_change_password') === 'true'
  const role = sessionStorage.getItem('role') || ''
  if (to.path !== '/login' && !authenticated) return '/login'
  if (to.path === '/login' && authenticated) return '/'
  if (authenticated && mustChangePassword && to.path !== '/change-password')
    return '/change-password'
  if (authenticated && !mustChangePassword && to.path === '/change-password') return '/'
  const roles = to.meta.roles as string[] | undefined
  if (roles && !roles.includes(role)) return '/'
  if (to.meta.sectionRoles) {
    const restricted: Record<string, string[]> = {
      docker: ['admin', 'operator'],
      services: ['admin', 'operator'],
      terminal: ['admin', 'operator'],
      files: ['admin', 'operator'],
      firewall: ['admin', 'operator'],
      audit: ['admin'],
      notifications: ['admin', 'operator'],
    }
    const allowed = restricted[String(to.params.section)]
    if (allowed && !allowed.includes(role)) return '/'
  }
})

export default router
