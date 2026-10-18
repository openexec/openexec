import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: 'approvals-d4.spec.ts',
  timeout: 30000,
  reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:13001', ...devices['Desktop Chrome'] },
  webServer: [
    {
      command: 'go run -buildvcs=false ../scripts/defect-inventory/d4-server',
      port: 18080,
      timeout: 120000,
    },
    {
      command: 'node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 13001 --strictPort',
      env: { VITE_API_TARGET: 'http://127.0.0.1:18080' },
      url: 'http://127.0.0.1:13001',
      timeout: 30000,
    },
  ],
})
