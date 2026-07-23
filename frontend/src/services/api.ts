import axios from 'axios'
const api = axios.create({ baseURL: '/api/v1', timeout: 15000 })
api.interceptors.request.use((c) => {
  const t = sessionStorage.getItem('access_token')
  if (t) c.headers.Authorization = `Bearer ${t}`
  return c
})
api.interceptors.response.use(
  (r) => r,
  (e) => {
    if (e.response?.status === 401) {
      sessionStorage.removeItem('access_token')
      if (location.pathname != '/login') location.href = '/login'
    }
    return Promise.reject(e)
  },
)
export default api
