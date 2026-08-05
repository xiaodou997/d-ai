import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      "@": resolve(__dirname, "src")
    }
  },
  server: {
    port: 6900,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:13000",
        changeOrigin: true
      },
      "/v1": {
        target: "http://127.0.0.1:13000",
        changeOrigin: true
      },
      "/internal": {
        target: "http://127.0.0.1:13000",
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: "../../cmd/server/frontend_dist",
    emptyOutDir: true
  }
});
