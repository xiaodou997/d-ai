import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

const devProxyTarget = process.env.DAI_DEV_PROXY_TARGET?.trim() || "http://127.0.0.1:19641";
const portalOutDir = process.env.DAI_PORTAL_OUT_DIR?.trim() || "../../cmd/server/frontend_dist";

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
      // Keep the browser's Host header so the backend same-origin guard sees
      // the local Portal origin (localhost:6900) instead of the proxy target.
      "/api": {
        target: devProxyTarget,
        changeOrigin: false
      },
      "/v1": {
        target: devProxyTarget,
        changeOrigin: false
      },
      "/internal": {
        target: devProxyTarget,
        changeOrigin: false
      },
      "/runtime": {
        target: devProxyTarget,
        changeOrigin: false
      }
    }
  },
  build: {
    outDir: portalOutDir,
    emptyOutDir: true
  }
});
