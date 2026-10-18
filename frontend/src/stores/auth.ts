import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import api from '../services/api'
export const useAuthStore = defineStore('auth', () => {
  const token = ref(sessionStorage.getItem('access_token') || '')
  const mustChangePassword = ref(sessionStorage.getItem('must_change_password') === 'true')
  const username = ref(sessionStorage.getItem('username') || '')
  const role = ref(sessionStorage.getItem('role') || '')
  const authenticated = computed(() => !!token.value)
  // The second-factor challenge lives only in memory: it is useless after 5 minutes anyway.
  const mfaToken = ref('')

  interface LoginResponse {
    access_token?: string
    must_change_password?: boolean
    username?: string
    role?: string
    mfa_required?: boolean
    mfa_token?: string
  }

  function storeSession(data: LoginResponse) {
    token.value = data.access_token ?? ''
    mustChangePassword.value = Boolean(data.must_change_password)
    username.value = data.username ?? ''
    role.value = data.role ?? ''
    mfaToken.value = ''
    sessionStorage.setItem('access_token', token.value)
    sessionStorage.setItem('must_change_password', String(mustChangePassword.value))
    sessionStorage.setItem('username', username.value)
    sessionStorage.setItem('role', role.value)
  }

  /** Returns true when the password was accepted but a second factor is required. */
  async function login(loginUsername: string, password: string): Promise<boolean> {
    const { data } = await api.post<LoginResponse>(
      '/auth/login',
      { username: loginUsername, password },
      { silent: true },
    )
    if (data.mfa_required && data.mfa_token) {
      mfaToken.value = data.mfa_token
      return true
    }
    storeSession(data)
    return false
  }

  async function verifySecondFactor(code: string) {
    const { data } = await api.post<LoginResponse>(
      '/auth/login/mfa',
      { mfa_token: mfaToken.value, code },
      { silent: true },
    )
    storeSession(data)
  }

  function cancelSecondFactor() {
    mfaToken.value = ''
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
    mfaToken,
    login,
    verifySecondFactor,
    cancelSecondFactor,
    changePassword,
    logout,
    clearSession,
  }
})
