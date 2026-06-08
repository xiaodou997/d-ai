import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// https://vitejs.dev/config/
export default defineConfig({
  root: __dirname,

  plugins: [
    vue()
  ],

  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '@unihub/ui': resolve(__dirname, '../../packages/ui'),
      '@unihub/billing': resolve(__dirname, '../../packages/billing'),
      'vue': resolve(__dirname, 'node_modules/vue'),
      'vue-router': resolve(__dirname, 'node_modules/vue-router'),
      'pinia': resolve(__dirname, 'node_modules/pinia'),
      'element-plus': resolve(__dirname, 'node_modules/element-plus'),
      '@element-plus/icons-vue': resolve(__dirname, 'node_modules/@element-plus/icons-vue'),
      'dayjs': resolve(__dirname, 'node_modules/dayjs'),
      'echarts': resolve(__dirname, 'node_modules/echarts'),
      'tailwindcss': resolve(__dirname, 'node_modules/tailwindcss/index.css')
    }
  },

  server: {
    port: 13012,
    host: '0.0.0.0',
    proxy: {
      // AI Gateway management API.
      '/api': {
        target: 'http://localhost:13010',
        changeOrigin: true
      },
      // JWT web runtime APIs, including streaming console chat.
      '/console': {
        target: 'http://localhost:13010',
        changeOrigin: true
      },
      // URM shared pages call URM APIs through the host app's /urm prefix.
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
    cssMinify: false,
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
