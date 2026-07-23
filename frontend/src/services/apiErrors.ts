export interface APIErrorDetail {
  code: string
  status?: number
  network: boolean
}

const eventName = 'mini-server:api-error'

export function emitAPIError(detail: APIErrorDetail) {
  window.dispatchEvent(new CustomEvent<APIErrorDetail>(eventName, { detail }))
}

export function onAPIError(listener: (detail: APIErrorDetail) => void) {
  const handler = (event: Event) => listener((event as CustomEvent<APIErrorDetail>).detail)
  window.addEventListener(eventName, handler)
  return () => window.removeEventListener(eventName, handler)
}
