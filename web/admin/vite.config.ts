import { defineConfig } from 'vite'

export default defineConfig({
  base: '/admin/',
  build: {
    outDir: '../../internal/httpapi/admin_dist',
    emptyOutDir: false,
    assetsDir: 'assets',
    sourcemap: false,
  },
})
