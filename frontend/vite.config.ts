import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'node:path'
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: resolve(__dirname, '../backend/cmd/mini-ubuntu-server/web'),
    emptyOutDir: false,
    // The backend CSP allows only font-src 'self', so fonts must never be inlined as data: URIs.
    assetsInlineLimit: (file) => (/\.woff2?$/.test(file) ? false : undefined),
  },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', ws: true },
    },
  },
})
