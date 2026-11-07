/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,ts}'],
  darkMode: ['selector', '[data-theme="dark"]'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'Segoe UI', 'system-ui', '-apple-system', 'Roboto', 'sans-serif'],
        // Bare `monospace` resolves to Courier New on Windows, so list the common system monospace fonts.
        mono: [
          'JetBrains Mono',
          'Cascadia Mono',
          'SF Mono',
          'Menlo',
          'Consolas',
          'DejaVu Sans Mono',
          'Liberation Mono',
          'ui-monospace',
          'monospace',
        ],
      },
    },
  },
  plugins: [],
}
