// Fonts are bundled from @fontsource-variable (latin + cyrillic subsets load on demand),
// so the panel never depends on an external CDN and works on isolated servers.
import '@fontsource-variable/geist'
import '@fontsource-variable/jetbrains-mono'

export const SANS_FONT = "'Geist Variable', 'Segoe UI', system-ui, -apple-system, sans-serif"
export const MONO_FONT =
  "'JetBrains Mono Variable', 'Cascadia Code', 'Ubuntu Mono', ui-monospace, monospace"

/** Resolves once the monospace face is usable, so canvas renderers measure real glyphs. */
export async function monoFontReady(size = 14): Promise<void> {
  try {
    await document.fonts.load(`${size}px 'JetBrains Mono Variable'`)
  } catch {
    // Fallback fonts from MONO_FONT are acceptable when loading fails.
  }
}
