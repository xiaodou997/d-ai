import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { federation } from '@module-federation/vite'
import { resolve } from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    federation({
      name: 'ai_customer_host',
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
    port: 13013,
    host: '0.0.0.0',
    proxy: {
      '/api': {
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
        // 文件命名格式，加上 [name] 方便你区分是哪个包
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]'
      }
    }
  }
})
