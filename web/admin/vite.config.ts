import { defineConfig } from 'vite'

export default defineConfig({
  base: '/admin/',
  publicDir: '../../internal/httpapi/assets',
  build: {
    outDir: '../../internal/httpapi/admin_dist',
    emptyOutDir: false,
    assetsDir: 'assets',
    sourcemap: false,
  },
})
