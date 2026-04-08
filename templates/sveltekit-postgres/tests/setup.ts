import { PostgreSqlContainer, type StartedPostgreSqlContainer } from 'testcontainers';
import { afterAll, beforeAll } from 'vitest';

let container: StartedPostgreSqlContainer | undefined;

beforeAll(async () => {
	container = await new PostgreSqlContainer('postgres:16-alpine')
		.withDatabase('app_test')
		.withUsername('app')
		.withPassword('app')
		.start();

	process.env.DATABASE_URL = container.getConnectionUri();

	// Migrations are applied lazily by the first helper that needs them.
	const { migrate } = await import('drizzle-orm/postgres-js/migrator');
	const { drizzle } = await import('drizzle-orm/postgres-js');
	const postgres = (await import('postgres')).default;
	const client = postgres(process.env.DATABASE_URL, { max: 1 });
	const db = drizzle(client);
	try {
		await migrate(db, { migrationsFolder: './drizzle' });
	} catch (err) {
		// If drizzle/ is empty (no migrations yet), skip — schema may be
		// applied by tests directly in early bootstrapping.
		if (!(err instanceof Error) || !/No migrations/.test(err.message)) {
			// Re-throw anything else.
			console.warn('[tests/setup] migrate warning:', err);
		}
	} finally {
		await client.end();
	}
}, 120_000);

afterAll(async () => {
	if (container) {
		await container.stop();
		container = undefined;
	}
});
