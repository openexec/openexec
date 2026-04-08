import { drizzle } from 'drizzle-orm/postgres-js';
import postgres from 'postgres';
import * as schema from './schema';

const connectionString =
	process.env.DATABASE_URL ?? 'postgres://app:app@localhost:5432/app';

// Single shared connection pool per process.
const client = postgres(connectionString, { max: 10 });

export const db = drizzle(client, { schema });
export { client as pg };
