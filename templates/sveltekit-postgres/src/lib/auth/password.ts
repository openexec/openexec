import { randomBytes, scrypt, timingSafeEqual } from 'node:crypto';
import { promisify } from 'node:util';

const scryptAsync = promisify(scrypt);

const KEY_LEN = 64;

export async function hashPassword(password: string): Promise<string> {
	const salt = randomBytes(16).toString('hex');
	const derived = (await scryptAsync(password, salt, KEY_LEN)) as Buffer;
	return `${salt}:${derived.toString('hex')}`;
}

export async function verifyPassword(password: string, stored: string): Promise<boolean> {
	const [salt, hex] = stored.split(':');
	if (!salt || !hex) return false;
	const derived = (await scryptAsync(password, salt, KEY_LEN)) as Buffer;
	const known = Buffer.from(hex, 'hex');
	if (known.length !== derived.length) return false;
	return timingSafeEqual(known, derived);
}
