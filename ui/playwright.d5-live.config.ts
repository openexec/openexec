import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  // Invoked only by the Go harness that supplies the live server and credentials.
  testMatch: 'knowledge-hub-live.journey.ts',
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: process.env.D5_BASE_URL,
    headless: true,
    extraHTTPHeaders: {
      Authorization: `Bearer ${process.env.D5_TOKEN}`,
      'X-OpenExec-Checkout-ID': process.env.D5_CHECKOUT!,
    },
  },
})
