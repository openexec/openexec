import type { RequestHandler } from '@sveltejs/kit';
import { ok } from '$lib/api/response';

export const GET: RequestHandler = async () => {
	return ok({ status: 'ok' });
};
