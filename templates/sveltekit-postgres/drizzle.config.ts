import type { Config } from 'drizzle-kit';

export default {
	schema: './src/lib/db/schema/index.ts',
	out: './drizzle',
	dialect: 'postgresql',
	dbCredentials: {
		url: process.env.DATABASE_URL ?? 'postgres://app:app@localhost:5432/app'
	},
	strict: true,
	verbose: true
} satisfies Config;
