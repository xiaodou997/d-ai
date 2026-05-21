import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { federation } from '@module-federation/vite'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// https://vitejs.dev/config/
export default defineConfig({
  root: __dirname,

  plugins: [
    vue(),
    federation({
      name: 'ai_admin_host',
      filename: 'remoteEntry.js',
      dts: false,
      shared: ['vue', 'vue-router', 'pinia', 'element-plus']
    })
  ],

  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },

  server: {
    port: 13011,
    host: '0.0.0.0',
    proxy: {
      '/admin': {
        target: 'http://localhost:13010',
        changeOrigin: true
      },
      '/api': {
        target: 'http://localhost:13010',
        changeOrigin: true
      },
      '/v1': {
        target: 'http://localhost:13010',
        changeOrigin: true
      },
      '/urm/v1': {
        target: 'http://localhost:6900',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/urm/, '/api')
      },
      '/urm/oauth2': {
        target: 'http://localhost:6900',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/urm/, '/api')
      }
    }
  },

  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 1500,

    rolldownOptions: {
      output: {
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]'
      }
    }
  }
})
