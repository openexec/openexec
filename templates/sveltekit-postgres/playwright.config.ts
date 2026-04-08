import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: './tests/e2e',
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: 1,
	use: {
		baseURL: process.env.PUBLIC_APP_URL ?? 'http://localhost:5173',
		trace: 'on-first-retry'
	}
});
