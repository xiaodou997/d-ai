import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import { resolve } from "node:path";

export default defineConfig({
  plugins: [vue()],
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
