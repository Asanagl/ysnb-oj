import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'

// dev proxy: the backend never runs on Windows (Linux-only sandbox); all
// backend interaction goes through the cloud server. Point OJ_DEV_API_TARGET
// at the deployment origin (scheme://host, no /api path) to develop against
// it; the default keeps a local backend workflow possible for contributors.
// ws:true is required — the judge verdict pushes ride WebSocket upgrades
// (/api/v1/ws), and without it dev pages never see live status changes.
const target = process.env.OJ_DEV_API_TARGET ?? 'http://127.0.0.1:18080'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target,
        changeOrigin: true,
        ws: true,
      },
    },
  },
  build: {
    chunkSizeWarningLimit: 1500,
  },
})
