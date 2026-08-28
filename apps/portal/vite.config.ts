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
      "/api": {
        target: devProxyTarget,
        changeOrigin: true
      },
      "/v1": {
        target: devProxyTarget,
        changeOrigin: true
      },
      "/internal": {
        target: devProxyTarget,
        changeOrigin: true
      },
      "/runtime": {
        target: devProxyTarget,
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: portalOutDir,
    emptyOutDir: true
  }
});
