import { drizzle } from 'drizzle-orm/postgres-js';
import { migrate } from 'drizzle-orm/postgres-js/migrator';
import postgres from 'postgres';

async function main() {
	const url = process.env.DATABASE_URL;
	if (!url) {
		throw new Error('DATABASE_URL is required');
	}
	const migrationClient = postgres(url, { max: 1 });
	const db = drizzle(migrationClient);
	await migrate(db, { migrationsFolder: './drizzle' });
	await migrationClient.end();
	console.log('Migrations applied.');
}

main().catch((err) => {
	console.error(err);
	process.exit(1);
});
