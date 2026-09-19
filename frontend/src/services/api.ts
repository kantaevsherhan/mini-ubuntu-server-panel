import axios from 'axios'
import { emitAPIError } from './apiErrors'

declare module 'axios' {
  interface AxiosRequestConfig {
    /** Background polling: do not show an error toast for this request. */
    silent?: boolean
  }
}

const api = axios.create({ baseURL: '/api/v1', timeout: 15000 })
api.interceptors.request.use((c) => {
  const t = sessionStorage.getItem('access_token')
  if (t) c.headers.Authorization = `Bearer ${t}`
  return c
})
api.interceptors.response.use(
  (r) => r,
  (e) => {
    const code = String(e.response?.data?.error || 'network_error')
    const message = e.response?.data?.message
    if (!e.config?.silent) {
      emitAPIError({
        code,
        message: typeof message === 'string' ? message : undefined,
        status: e.response?.status,
        network: !e.response,
      })
    }
    if (e.response?.status === 401) {
      sessionStorage.removeItem('access_token')
      sessionStorage.removeItem('must_change_password')
      if (location.pathname != '/login') location.href = '/login'
    }
    return Promise.reject(e)
  },
)
export default api
