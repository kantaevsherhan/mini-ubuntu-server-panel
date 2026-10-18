import { readonly, ref } from 'vue'

/**
 * Server reachability as seen by the browser. Any HTTP response (even an error status)
 * proves the backend is reachable; only transport failures and browser offline events
 * mark the connection as lost. While lost, a lightweight health probe runs until the
 * server answers again, so pages without polling still recover the indicator.
 */
const PROBE_INTERVAL_MS = 10_000
const online = ref(typeof navigator === 'undefined' ? true : navigator.onLine)
let probeTimer: number | undefined

async function probe() {
  try {
    const response = await fetch('/api/v1/health', { cache: 'no-store' })
    if (response.ok) markReachable()
  } catch {
    // Still unreachable; the next probe retries.
  }
}

export function markReachable() {
  online.value = true
  window.clearInterval(probeTimer)
  probeTimer = undefined
}

export function markUnreachable() {
  online.value = false
  probeTimer ??= window.setInterval(probe, PROBE_INTERVAL_MS)
}

if (typeof window !== 'undefined') {
  window.addEventListener('offline', markUnreachable)
  // Regaining a network does not prove the server is up; probe instead of trusting it.
  window.addEventListener('online', () => void probe())
}

export const serverOnline = readonly(online)
