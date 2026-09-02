import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

// dev proxy keeps origin simple; production serves static via nginx
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:18080',
        changeOrigin: true,
      },
    },
  },
  build: {
    chunkSizeWarningLimit: 1500,
  },
})
