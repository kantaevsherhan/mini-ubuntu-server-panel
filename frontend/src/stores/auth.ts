import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import api from '../services/api'
export const useAuthStore = defineStore('auth', () => {
  const token = ref(sessionStorage.getItem('access_token') || '')
  const mustChangePassword = ref(sessionStorage.getItem('must_change_password') === 'true')
  const username = ref(sessionStorage.getItem('username') || '')
  const role = ref(sessionStorage.getItem('role') || '')
  const authenticated = computed(() => !!token.value)
  async function login(loginUsername: string, password: string) {
    const { data } = await api.post('/auth/login', { username: loginUsername, password })
    token.value = data.access_token
    mustChangePassword.value = Boolean(data.must_change_password)
    username.value = data.username
    role.value = data.role
    sessionStorage.setItem('access_token', token.value)
    sessionStorage.setItem('must_change_password', String(mustChangePassword.value))
    sessionStorage.setItem('username', username.value)
    sessionStorage.setItem('role', role.value)
  }
  async function changePassword(currentPassword: string, newPassword: string) {
    await api.post('/auth/password', {
      current_password: currentPassword,
      new_password: newPassword,
    })
    mustChangePassword.value = false
    username.value = ''
    role.value = ''
    sessionStorage.setItem('must_change_password', 'false')
  }
  async function logout() {
    try {
      if (token.value) await api.post('/auth/logout')
    } finally {
      clearSession()
    }
  }
  function clearSession() {
    token.value = ''
    mustChangePassword.value = false
    sessionStorage.removeItem('access_token')
    sessionStorage.removeItem('must_change_password')
    sessionStorage.removeItem('username')
    sessionStorage.removeItem('role')
  }
  return {
    token,
    authenticated,
    mustChangePassword,
    username,
    role,
    login,
    changePassword,
    logout,
    clearSession,
  }
})
