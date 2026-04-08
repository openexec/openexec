import type { RequestHandler } from '@sveltejs/kit';
import { lucia } from '$lib/auth/lucia';
import { error, ok } from '$lib/api/response';

export const POST: RequestHandler = async ({ locals, cookies }) => {
	if (!locals.session) {
		return error('not authenticated', 401, 'not_authenticated');
	}

	await lucia.invalidateSession(locals.session.id);
	const sessionCookie = lucia.createBlankSessionCookie();
	cookies.set(sessionCookie.name, sessionCookie.value, {
		path: '.',
		...sessionCookie.attributes
	});

	return ok({ success: true });
};
