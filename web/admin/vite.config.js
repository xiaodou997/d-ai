import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// https://vitejs.dev/config/
export default defineConfig({
  root: __dirname,

  plugins: [vue()],

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
      '/v1': {
        target: 'http://localhost:13010',
        changeOrigin: true
      },
      '/urm': {
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
        codeSplitting: {
          groups: [
            {
              name: 'echarts',
              test: /node_modules[\\/]echarts/,
              priority: 20
            },
            {
              name: 'ui',
              test: /node_modules[\\/]element-plus/,
              priority: 15
            },
            {
              name: 'framework',
              test: /node_modules[\\/]vue/,
              priority: 10
            },
            {
              name: 'vendor',
              test: /node_modules/,
              priority: 5
            },
            {
              name: 'common',
              minShareCount: 2,
              minSize: 10000,
              priority: 1
            }
          ]
        }
      }
    }
  }
})
