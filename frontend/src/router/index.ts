import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../pages/LoginPage.vue') },
    {
      path: '/',
      component: () => import('../layouts/AppLayout.vue'),
      children: [
        { path: '', component: () => import('../pages/DashboardPage.vue') },
        { path: 'users', component: () => import('../pages/UsersPage.vue') },
        { path: 'settings', component: () => import('../pages/SettingsPage.vue') },
        { path: ':section', component: () => import('../pages/PlaceholderPage.vue') },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const authenticated = Boolean(sessionStorage.getItem('access_token'))
  if (to.path !== '/login' && !authenticated) return '/login'
  if (to.path === '/login' && authenticated) return '/'
})

export default router
