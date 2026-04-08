import type { RequestHandler } from '@sveltejs/kit';
import { eq } from 'drizzle-orm';
import { db } from '$lib/db/client';
import { users } from '$lib/db/schema';
import { lucia } from '$lib/auth/lucia';
import { hashPassword } from '$lib/auth/password';
import { error, ok } from '$lib/api/response';

export const POST: RequestHandler = async ({ request, cookies }) => {
	let body: { email?: string; password?: string };
	try {
		body = await request.json();
	} catch {
		return error('invalid json body', 400, 'invalid_json');
	}

	const email = body.email?.trim().toLowerCase();
	const password = body.password;

	if (!email || !password) {
		return error('email and password are required', 400, 'missing_fields');
	}
	if (password.length < 8) {
		return error('password must be at least 8 characters', 400, 'weak_password');
	}

	const existing = await db.select().from(users).where(eq(users.email, email)).limit(1);
	if (existing.length > 0) {
		return error('email already registered', 409, 'email_taken');
	}

	const passwordHash = await hashPassword(password);
	const [created] = await db
		.insert(users)
		.values({ email, passwordHash })
		.returning({ id: users.id, email: users.email });

	const session = await lucia.createSession(created.id, {});
	const sessionCookie = lucia.createSessionCookie(session.id);
	cookies.set(sessionCookie.name, sessionCookie.value, {
		path: '.',
		...sessionCookie.attributes
	});

	return ok({ user: created });
};
