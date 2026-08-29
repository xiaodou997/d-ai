import { defineConfig, devices } from "@playwright/test";

const externalBaseURL = process.env.DAI_E2E_BASE_URL?.trim();
const baseURL = externalBaseURL || "http://127.0.0.1:6900";
const browserPath = process.env.DAI_E2E_BROWSER_PATH?.trim();
const launchOptions = browserPath ? { launchOptions: { executablePath: browserPath } } : {};

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.spec.ts",
  outputDir: "test-results/e2e",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    serviceWorkers: "block"
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], ...launchOptions }
    },
    {
      name: "mobile-chrome",
      use: { ...devices["Pixel 7"], ...launchOptions }
    }
  ],
  ...(externalBaseURL
    ? {}
    : {
        webServer: {
          command: "bun run dev:frontend -- --host 127.0.0.1",
          url: "http://127.0.0.1:6900/login",
          reuseExistingServer: !process.env.CI,
          timeout: 120_000,
          stdout: "pipe" as const,
          stderr: "pipe" as const
        }
      })
});
