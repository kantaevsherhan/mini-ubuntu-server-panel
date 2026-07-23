import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import api from '../services/api'
export const useAuthStore = defineStore('auth', () => {
  const token = ref(sessionStorage.getItem('access_token') || '')
  const mustChangePassword = ref(sessionStorage.getItem('must_change_password') === 'true')
  const authenticated = computed(() => !!token.value)
  async function login(username: string, password: string) {
    const { data } = await api.post('/auth/login', { username, password })
    token.value = data.access_token
    mustChangePassword.value = Boolean(data.must_change_password)
    sessionStorage.setItem('access_token', token.value)
    sessionStorage.setItem('must_change_password', String(mustChangePassword.value))
  }
  async function changePassword(currentPassword: string, newPassword: string) {
    await api.post('/auth/password', {
      current_password: currentPassword,
      new_password: newPassword,
    })
    mustChangePassword.value = false
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
  }
  return { token, authenticated, mustChangePassword, login, changePassword, logout, clearSession }
})
