import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const adminProxy = process.env.INLETS_ADMIN_PROXY ?? 'http://127.0.0.1:9090'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: adminProxy,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../internal/server/admin/static/dist',
    emptyOutDir: true,
  },
})
