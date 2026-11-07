import { computed, onBeforeUnmount, ref } from 'vue'

/**
 * Row heights for virtual-scrolled tables (`class="vt"` / `"vt vt-tall"`).
 * PrimeVue positions virtual rows by `itemSize`, so it must equal the rendered row height;
 * these values mirror `--vt-row` in main.css. Below 1024px the root font is larger, so rows are too.
 */
const desktopQuery = '(min-width: 1024px)'

export function useTableRow() {
  const media = window.matchMedia(desktopQuery)
  const desktop = ref(media.matches)
  const update = (event: MediaQueryListEvent) => (desktop.value = event.matches)
  media.addEventListener('change', update)
  onBeforeUnmount(() => media.removeEventListener('change', update))
  return {
    rowSize: computed(() => (desktop.value ? 44 : 48)),
    tallRowSize: computed(() => (desktop.value ? 52 : 56)),
  }
}
