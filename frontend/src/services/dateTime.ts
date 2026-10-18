import moment, { type MomentInput } from 'moment'
import 'moment/dist/locale/ru.js'
import type { Locale } from '../stores/preferences'

const formats: Record<Locale, string> = {
  ru: 'DD.MM.YYYY HH:mm',
  en: 'MM/DD/YYYY h:mm A',
}

export function formatDateTime(value: MomentInput, locale: Locale): string {
  if (!value) return '—'
  const date = moment(value)
  if (!date.isValid()) return '—'
  return date.locale(locale).format(formats[locale])
}

export function formatRelativeTime(value: MomentInput, locale: Locale): string {
  if (!value) return '—'
  const date = moment(value)
  if (!date.isValid()) return '—'
  return date.locale(locale).fromNow()
}
