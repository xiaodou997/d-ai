import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

export default defineConfig({
  // Build commands run from the repository root; the Portal owns the Vite root.
  root: import.meta.dirname,
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "src")
    }
  },
  server: {
    port: 6900,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:19641",
        changeOrigin: true
      },
      "/v1": {
        target: "http://127.0.0.1:19641",
        changeOrigin: true
      },
      "/internal": {
        target: "http://127.0.0.1:19641",
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: "../../cmd/server/frontend_dist",
    emptyOutDir: true
  }
});
