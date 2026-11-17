import { describe, expect, it } from "vitest";
import { translate } from "@/lib/i18n";
import enMessages from "../../../lang/en.json";
import ptMessages from "../../../lang/pt-BR.json";
import { type ErrorLike, errorText } from "./errorText";

function flat(tree: unknown, prefix = ""): Record<string, string> {
	const out: Record<string, string> = {};
	if (typeof tree !== "object" || tree === null) return out;
	for (const [k, v] of Object.entries(tree)) {
		if (typeof v === "string") out[`${prefix}${k}`] = v;
		else Object.assign(out, flat(v, `${prefix}${k}.`));
	}
	return out;
}

const ctx = {
	itemName: (id: string) => `<${id}>`,
	time: (f: number) => `${f}f`,
};

function run(locale: "en" | "pt-BR", error: ErrorLike) {
	const messages = flat(locale === "en" ? enMessages : ptMessages);
	return errorText(
		(key, params) => translate(messages, locale, key, params),
		error,
		ctx,
	);
}

describe("errorText (VV-126)", () => {
	it("words E_OVERLAP with the conflicting item and the next free frame", () => {
		const error = {
			code: "E_OVERLAP",
			message: "Item itm_a would occupy frames 0-9",
			hint: "Use start",
			details: { conflicting_item: "itm_b", next_free: 90 },
		};
		const pt = run("pt-BR", error);
		expect(pt.localized).toBe(true);
		expect(pt.message).toBe("Ficaria sobre <itm_b>.");
		expect(pt.hint).toContain("90f");
		expect(run("en", error).message).toBe("It would overlap <itm_b>.");
	});

	it("says no free position when next_free is null", () => {
		const text = run("en", {
			code: "E_OVERLAP",
			message: "x",
			details: { conflicting_item: "itm_b", next_free: null },
		});
		expect(text.hint).toMatch(/No free position/);
	});

	it("words E_VALIDATION by accepted values", () => {
		expect(
			run("en", {
				code: "E_VALIDATION",
				message: "m",
				details: { allowed: ["a", "b"] },
			}).message,
		).toBe("The accepted values are: a, b.");
	});

	it("words the limits and the rate limit", () => {
		expect(
			run("en", {
				code: "E_QUEUE_FULL",
				message: "m",
				details: { limit: "exports", value: 1, ceiling: 1 },
			}).message,
		).toBe("Limit reached for exports: 1 of 1 in use.");
		expect(
			run("pt-BR", {
				code: "E_RATE_LIMITED",
				message: "m",
				details: { retry_after: 7 },
			}).message,
		).toContain("7 s");
	});

	it("words E_OUT_OF_SOURCE and E_ID_EXISTS from details", () => {
		expect(
			run("en", {
				code: "E_OUT_OF_SOURCE",
				message: "m",
				details: { available: { max_duration: 30 } },
			}).message,
		).toBe("The source has only 30f");
		expect(
			run("en", { code: "E_ID_EXISTS", message: "m", details: { id: "itm_x" } })
				.message,
		).toBe("The id itm_x is already in use.");
	});

	it("words the codes whose message the code fully determines", () => {
		expect(
			run("pt-BR", { code: "E_NOTHING_TO_UNDO", message: "m" }).message,
		).toBe("Não há passo para desfazer.");
		for (const code of [
			"E_NOTHING_TO_REDO",
			"E_ASSET_NOT_READY",
			"E_UNAUTHORIZED",
			"E_FORBIDDEN",
			"E_PATH_NOT_ALLOWED",
			"E_FILE_NOT_FOUND",
			"E_FFMPEG_MISSING",
			"E_NO_ENCODER",
			"E_WORKSPACE_LOCKED",
			"E_PAYLOAD_TOO_LARGE",
			"E_CANCELLED",
		]) {
			expect(run("pt-BR", { code, message: "English" }).localized).toBe(true);
		}
	});

	it("falls back to the server English without an entry or without the details", () => {
		for (const error of [
			{ code: "E_NOT_FOUND", message: "Item itm_z not found", hint: "h" },
			{ code: "E_OVERLAP", message: "Item overlaps", hint: "h" },
			{ code: "E_VALIDATION", message: "bad", hint: "h", details: {} },
			{ code: "E_UNKNOWN_FUTURE", message: "bad", hint: "h" },
		]) {
			const text = run("pt-BR", error);
			expect(text).toEqual({
				message: error.message,
				hint: "h",
				localized: false,
			});
		}
	});
});
