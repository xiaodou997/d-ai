import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],

  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },

  server: {
    port: 13012,
    host: '0.0.0.0',
    proxy: {
      '/urm': {
        target: 'http://localhost:13010',
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
        },
        // 文件命名格式，加上 [name] 方便你区分是哪个包
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]'
      }
    }
  }
})
