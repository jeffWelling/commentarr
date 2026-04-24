import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:7878',
      '/healthz': 'http://localhost:7878',
      '/metrics': 'http://localhost:7878',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
