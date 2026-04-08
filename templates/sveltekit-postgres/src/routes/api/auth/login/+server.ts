import type { RequestHandler } from '@sveltejs/kit';
import { eq } from 'drizzle-orm';
import { db } from '$lib/db/client';
import { users } from '$lib/db/schema';
import { lucia } from '$lib/auth/lucia';
import { verifyPassword } from '$lib/auth/password';
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

	const [user] = await db.select().from(users).where(eq(users.email, email)).limit(1);
	if (!user) {
		return error('invalid credentials', 401, 'invalid_credentials');
	}

	const valid = await verifyPassword(password, user.passwordHash);
	if (!valid) {
		return error('invalid credentials', 401, 'invalid_credentials');
	}

	const session = await lucia.createSession(user.id, {});
	const sessionCookie = lucia.createSessionCookie(session.id);
	cookies.set(sessionCookie.name, sessionCookie.value, {
		path: '.',
		...sessionCookie.attributes
	});

	return ok({ user: { id: user.id, email: user.email } });
};
