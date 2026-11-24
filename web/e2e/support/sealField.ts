import {
	createCipheriv,
	createDecipheriv,
	hkdfSync,
	randomBytes,
} from "node:crypto";
import { readFileSync } from "node:fs";
import type { DatabaseSync } from "node:sqlite";
import { recoveryKeyPath } from "./account";

// A spec that writes a row straight into the database must write a sealed column the way the store does: with an
// account a clear value there is refused (T-105), so the whole list that holds the row answers an error. This
// follows SDD-005 section 5.17.1 and SDD-007 section 3.9 item 2. It opens the data key with the recovery key of the
// throwaway account, which needs no Argon2id: node:crypto has it only from Node 24.7, and CI does not pin Node.
export type FieldSealer = (
	table: string,
	column: string,
	rowKey: string,
	plain: string,
) => string;

const NONCE = 12;
const TAG = 16;
const CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";

function decodeRecoveryKey(text: string): Buffer {
	const out: number[] = [];
	let bits = 0;
	let value = 0;
	for (const char of text.replace(/[-\s]/g, "")) {
		value = (value << 5) | CROCKFORD.indexOf(char);
		bits += 5;
		if (bits >= 8) {
			bits -= 8;
			out.push((value >> bits) & 0xff);
			value &= (1 << bits) - 1;
		}
	}
	return Buffer.from(out);
}

function dataKey(wrapped: Buffer, home: string): Buffer {
	const recovery = decodeRecoveryKey(
		readFileSync(recoveryKeyPath(home), "utf8").trim(),
	);
	const kek = Buffer.from(
		hkdfSync(
			"sha256",
			recovery,
			Buffer.alloc(0),
			"stuarty-account-recovery-v1",
			32,
		),
	);
	const decipher = createDecipheriv(
		"aes-256-gcm",
		kek,
		wrapped.subarray(0, NONCE),
	);
	decipher.setAAD(Buffer.from("stuarty/account/v1\0data-by-recovery"));
	decipher.setAuthTag(wrapped.subarray(wrapped.length - TAG));
	return Buffer.concat([
		decipher.update(wrapped.subarray(NONCE, wrapped.length - TAG)),
		decipher.final(),
	]);
}

// fieldSealer seals for the account of this database; with no account the columns hold clear text.
export function fieldSealer(db: DatabaseSync, home: string): FieldSealer {
	const row = db
		.prepare("SELECT key_by_recovery FROM accounts WHERE id = 1")
		.get() as { key_by_recovery: Uint8Array } | undefined;
	if (row === undefined) return (_table, _column, _rowKey, plain) => plain;

	const key = dataKey(Buffer.from(row.key_by_recovery), home);
	return (table, column, rowKey, plain) => {
		const nonce = randomBytes(NONCE);
		const cipher = createCipheriv("aes-256-gcm", key, nonce);
		cipher.setAAD(
			Buffer.from(`stuarty/field/v1\0${table}.${column}\0${rowKey}`),
		);
		const body = Buffer.concat([cipher.update(plain, "utf8"), cipher.final()]);
		return `s1.${Buffer.concat([nonce, body, cipher.getAuthTag()]).toString("base64url")}`;
	};
}
