import { ref, watch } from 'vue'
import { defineStore } from 'pinia'
import Aura from '@primeuix/themes/aura'
import Lara from '@primeuix/themes/lara'
import { usePreset, updatePrimaryPalette } from '@primeuix/themes'

export type Locale = 'ru' | 'en'
export type ThemePreset = 'aura' | 'lara'
export type ColorMode = 'dark' | 'light'
export const usePreferencesStore = defineStore('preferences', () => {
  const locale = ref<Locale>((localStorage.getItem('locale') as Locale) || 'ru')
  const preset = ref<ThemePreset>((localStorage.getItem('preset') as ThemePreset) || 'aura')
  const mode = ref<ColorMode>((localStorage.getItem('color-mode') as ColorMode) || 'dark')
  const accent = ref(localStorage.getItem('accent') || 'emerald')
  const sidebarWidth = ref(Number(localStorage.getItem('sidebar-width') || 240))
  const palettes: Record<string, Record<number, string>> = {
    emerald: {
      50: '{emerald.50}',
      100: '{emerald.100}',
      200: '{emerald.200}',
      300: '{emerald.300}',
      400: '{emerald.400}',
      500: '{emerald.500}',
      600: '{emerald.600}',
      700: '{emerald.700}',
      800: '{emerald.800}',
      900: '{emerald.900}',
      950: '{emerald.950}',
    },
    blue: {
      50: '{blue.50}',
      100: '{blue.100}',
      200: '{blue.200}',
      300: '{blue.300}',
      400: '{blue.400}',
      500: '{blue.500}',
      600: '{blue.600}',
      700: '{blue.700}',
      800: '{blue.800}',
      900: '{blue.900}',
      950: '{blue.950}',
    },
    violet: {
      50: '{violet.50}',
      100: '{violet.100}',
      200: '{violet.200}',
      300: '{violet.300}',
      400: '{violet.400}',
      500: '{violet.500}',
      600: '{violet.600}',
      700: '{violet.700}',
      800: '{violet.800}',
      900: '{violet.900}',
      950: '{violet.950}',
    },
  }
  function apply() {
    document.documentElement.dataset.theme = mode.value
    usePreset(preset.value === 'lara' ? Lara : Aura)
    updatePrimaryPalette(palettes[accent.value] || palettes.emerald)
  }
  watch(
    [locale, preset, mode, accent, sidebarWidth],
    () => {
      localStorage.setItem('locale', locale.value)
      localStorage.setItem('preset', preset.value)
      localStorage.setItem('color-mode', mode.value)
      localStorage.setItem('accent', accent.value)
      localStorage.setItem('sidebar-width', String(sidebarWidth.value))
      apply()
    },
    { immediate: true },
  )
  return { locale, preset, mode, accent, sidebarWidth, apply }
})
