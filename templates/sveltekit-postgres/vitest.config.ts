import { defineConfig } from 'vitest/config';
import { sveltekit } from '@sveltejs/kit/vite';

export default defineConfig({
	plugins: [sveltekit()],
	test: {
		workspace: [
			{
				extends: true,
				test: {
					name: 'unit',
					include: ['src/**/*.{test,spec}.ts'],
					environment: 'node'
				}
			},
			{
				extends: true,
				test: {
					name: 'contract',
					include: ['tests/contracts/**/*.test.ts'],
					environment: 'node',
					setupFiles: ['tests/setup.ts'],
					fileParallelism: false,
					sequence: { concurrent: false },
					testTimeout: 30_000,
					hookTimeout: 120_000
				}
			}
		]
	}
});
