import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import { resolve } from "node:path";

export default defineConfig({
  plugins: [vue()],
  // Tests run from the repository root, but the Portal owns the Vite root, so
  // its public assets must be pointed at explicitly. Without this, components
  // referencing root-absolute assets such as /brand/dai-logo.png fail to
  // resolve here while building fine in the real app.
  publicDir: resolve(import.meta.dirname, "apps/portal/public"),
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "apps/portal/src")
    }
  },
  test: {
    environment: "happy-dom",
    include: ["apps/portal/**/*.test.ts"],
    restoreMocks: true
  }
});
