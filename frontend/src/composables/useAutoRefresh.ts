import { onBeforeUnmount, onMounted, ref } from 'vue'

/**
 * Polls `task` only while the component is mounted and the browser tab is visible.
 * Requests never overlap, a hidden tab pauses polling, and returning to the tab refreshes at once,
 * so an idle panel puts no load on the server. `background` is true for timer-driven runs,
 * which should pass `{ silent: true }` to the API so failures do not spam toasts.
 */
export function useAutoRefresh(
  task: (background: boolean) => Promise<unknown>,
  intervalMs: number,
) {
  const enabled = ref(localStorage.getItem('auto-refresh') !== 'false')
  const refreshing = ref(false)
  let timer: number | undefined
  let active = false

  async function run(background = false) {
    if (refreshing.value) return
    refreshing.value = true
    try {
      await task(background)
    } catch {
      // Foreground errors are surfaced by the API interceptor; keep polling.
    } finally {
      refreshing.value = false
    }
  }

  function schedule() {
    window.clearTimeout(timer)
    if (!active || !enabled.value || document.hidden) return
    timer = window.setTimeout(async () => {
      await run(true)
      schedule()
    }, intervalMs)
  }

  function onVisibility() {
    if (!document.hidden && enabled.value) void run(true)
    schedule()
  }

  function toggle() {
    enabled.value = !enabled.value
    localStorage.setItem('auto-refresh', String(enabled.value))
    schedule()
  }

  onMounted(() => {
    active = true
    document.addEventListener('visibilitychange', onVisibility)
    void run(false).then(schedule)
  })

  onBeforeUnmount(() => {
    active = false
    window.clearTimeout(timer)
    document.removeEventListener('visibilitychange', onVisibility)
  })

  return { enabled, refreshing, refresh: () => run(false), toggle }
}
