import { sql } from 'drizzle-orm';
import { db } from '../src/lib/db/client';
import { users } from '../src/lib/db/schema';
import { hashPassword } from '../src/lib/auth/password';

export interface TestUser {
	id: string;
	email: string;
	password: string;
}

export async function createTestUser(
	overrides: Partial<{ email: string; password: string }> = {}
): Promise<TestUser> {
	const email = overrides.email ?? `user-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@example.test`;
	const password = overrides.password ?? 'correct-horse-battery';
	const passwordHash = await hashPassword(password);
	const [row] = await db
		.insert(users)
		.values({ email, passwordHash })
		.returning({ id: users.id, email: users.email });
	return { id: row.id, email: row.email, password };
}

export interface ApiCallOptions {
	method?: string;
	body?: unknown;
	headers?: Record<string, string>;
	baseUrl?: string;
}

export async function apiCall(path: string, opts: ApiCallOptions = {}): Promise<Response> {
	const baseUrl = opts.baseUrl ?? process.env.PUBLIC_APP_URL ?? 'http://localhost:5173';
	const url = path.startsWith('http') ? path : `${baseUrl}${path}`;
	const headers: Record<string, string> = { ...(opts.headers ?? {}) };
	let body: BodyInit | undefined;
	if (opts.body !== undefined) {
		headers['content-type'] ??= 'application/json';
		body = JSON.stringify(opts.body);
	}
	return fetch(url, { method: opts.method ?? 'GET', headers, body });
}

export async function resetDb(): Promise<void> {
	// Truncate all user tables in the public schema; cascades through FKs.
	await db.execute(sql`
		DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN
				SELECT tablename FROM pg_tables
				WHERE schemaname = 'public'
				  AND tablename NOT LIKE '__drizzle%'
			LOOP
				EXECUTE 'TRUNCATE TABLE ' || quote_ident(r.tablename) || ' RESTART IDENTITY CASCADE';
			END LOOP;
		END $$;
	`);
}
