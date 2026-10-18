import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'node:path'
export default defineConfig({
  plugins: [vue()],
  resolve: {
    // One ESM moment instance: the UMD entry plus a UMD locale were bundled twice,
    // so the Russian locale registered on a copy the app never used.
    alias: [{ find: /^moment$/, replacement: 'moment/dist/moment.js' }],
  },
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
