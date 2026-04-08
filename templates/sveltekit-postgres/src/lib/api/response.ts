export function json<T>(data: T, init: ResponseInit = {}): Response {
	return new Response(JSON.stringify(data), {
		...init,
		headers: {
			'content-type': 'application/json',
			...(init.headers ?? {})
		}
	});
}

export function ok<T>(data: T): Response {
	return json(data, { status: 200 });
}

export function error(message: string, status = 400, code?: string): Response {
	return json({ error: { message, code: code ?? null } }, { status });
}
