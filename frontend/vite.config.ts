import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // dev proxy: `npm run dev` (port 5173) tetap bisa panggil /api tanpa lewat nginx.
    // Di docker-compose ini gak kepake karena nginx yang proxy /api ke backend.
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
