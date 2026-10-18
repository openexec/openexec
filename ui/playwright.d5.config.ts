import { defineConfig } from '@playwright/test'

// Isolated UI journey: deterministic HTTP responses, no owner project or providers.
export default defineConfig({
  testDir: './e2e',
  testMatch: 'knowledge-hub.spec.ts',
  workers: 1,
  reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:3195', headless: true },
  webServer: {
    command: 'node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 3195 --strictPort',
    url: 'http://127.0.0.1:3195',
    reuseExistingServer: false,
  },
})
