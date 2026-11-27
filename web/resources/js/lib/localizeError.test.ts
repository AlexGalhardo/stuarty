import { describe, expect, it } from "vitest";
import { translate } from "@/lib/i18n";
import enMessages from "../../lang/en.json";
import ptMessages from "../../lang/pt-BR.json";
import {
	type ErrorLike,
	errorHeadline,
	localizeCaptionProblem,
	localizeError,
	localizeExportCheck,
	localizeJobMessage,
	localizeLintFinding,
	localizePostIssue,
} from "./localizeError";

function flat(tree: unknown, prefix = ""): Record<string, string> {
	const out: Record<string, string> = {};
	if (typeof tree !== "object" || tree === null) return out;
	for (const [k, v] of Object.entries(tree)) {
		if (typeof v === "string") out[`${prefix}${k}`] = v;
		else Object.assign(out, flat(v, `${prefix}${k}.`));
	}
	return out;
}

const enFlat = flat(enMessages);
const ptFlat = flat(ptMessages);

function makeT(locale: "en" | "pt-BR") {
	const messages = locale === "en" ? enFlat : ptFlat;
	return (key: string, params?: Record<string, string | number>) =>
		translate(messages, locale, key, params);
}

const ctx = {
	itemName: (id: string) => `<${id}>`,
	time: (frame: number) => `${frame}f`,
};

describe("localizeError", () => {
	it("localizes E_EXPORT_FAILED with reason encoder_failed and encoder alias in pt-BR", () => {
		const t = makeT("pt-BR");
		for (const reason of ["encoder_failed", "encoder"]) {
			const res = localizeError(
				t,
				{
					code: "E_EXPORT_FAILED",
					message: "The video encoder failed.",
					hint: 'Run "stuarty doctor" and try the export again.',
					details: { reason },
				},
				{ locale: "pt-BR" },
			);
			expect(res.localized).toBe(true);
			expect(res.headline).toBe("Falha na exportação");
			expect(res.message).toContain("codificador de vídeo");
			expect(res.hint).toContain("stuarty doctor");
			expect(res.message).not.toContain("encoder failed");
		}
	});

	it("localizes E_EXPORT_FAILED size_limit and webp_unavailable in pt-BR", () => {
		const t = makeT("pt-BR");
		const sizeLimit = localizeError(
			t,
			{
				code: "E_EXPORT_FAILED",
				message: "Too large",
				details: { reason: "size_limit" },
			},
			{ locale: "pt-BR" },
		);
		expect(sizeLimit.localized).toBe(true);
		expect(sizeLimit.message).toContain("limite de tamanho");
		expect(sizeLimit.hint).toContain("STUARTY_MAX_EXPORT_BYTES");

		const webp = localizeError(
			t,
			{
				code: "E_EXPORT_FAILED",
				message: "No libwebp",
				details: { reason: "webp_unavailable" },
			},
			{ locale: "pt-BR" },
		);
		expect(webp.localized).toBe(true);
		expect(webp.message).toContain("WebP");
	});

	it("localizes E_PROVIDER and E_PUBLISH sub-cases via details.kind", () => {
		const t = makeT("pt-BR");
		const provider = localizeError(
			t,
			{
				code: "E_PROVIDER",
				message: "Rate limited",
				details: { kind: "rate_limited" },
			},
			{ locale: "pt-BR" },
		);
		expect(provider.localized).toBe(true);
		expect(provider.message).toContain("limite de requisições");

		const publish = localizeError(
			t,
			{
				code: "E_PUBLISH",
				message: "Quota reached",
				details: { kind: "quota" },
			},
			{ locale: "pt-BR" },
		);
		expect(publish.localized).toBe(true);
		expect(publish.message).toContain("cota");
	});

	it("localizes E_AGENT_UNAVAILABLE, E_CAPTURE_UNAVAILABLE, E_CAPTURE_FAILED, and E_COST_CEILING", () => {
		const t = makeT("pt-BR");
		const cases: ErrorLike[] = [
			{
				code: "E_AGENT_UNAVAILABLE",
				message: "Sign in",
				details: { reason: "sign_in_required" },
			},
			{
				code: "E_CAPTURE_UNAVAILABLE",
				message: "Windows only",
				details: { reason: "platform" },
			},
			{
				code: "E_CAPTURE_FAILED",
				message: "No frames",
				details: { reason: "no_frames" },
			},
			{
				code: "E_COST_CEILING",
				message: "Over ceiling",
				details: { reason: "per_day" },
			},
		];
		for (const err of cases) {
			const res = localizeError(t, err, { locale: "pt-BR" });
			expect(res.localized).toBe(true);
			expect(res.message).not.toBe(err.message);
		}
	});

	it("localizes every E_* error code in en.json without falling back to raw English", () => {
		const tEn = makeT("en");
		const tPt = makeT("pt-BR");
		const codes = Object.keys(enMessages.ui.errors.message);
		expect(codes.length).toBeGreaterThanOrEqual(59);
		for (const code of codes) {
			const pt = localizeError(
				tPt,
				{ code, message: "RAW_ENGLISH_SENTINEL", hint: "RAW_HINT" },
				{ locale: "pt-BR", ctx },
			);
			expect(pt.localized, `pt-BR ${code}`).toBe(true);
			expect(pt.message, `pt-BR ${code}`).not.toBe("RAW_ENGLISH_SENTINEL");
			expect(errorHeadline(tPt, code), `pt-BR headline ${code}`).not.toBe(
				tPt("ui.editor.errors.unknown"),
			);
			expect(errorHeadline(tEn, code), `en headline ${code}`).not.toBe(
				tEn("ui.editor.errors.unknown"),
			);
		}
	});

	it("preserves raw English fallback only for truly unknown future codes", () => {
		const t = makeT("pt-BR");
		const res = localizeError(
			t,
			{ code: "E_UNKNOWN_FUTURE", message: "Something new", hint: "Try later" },
			{ locale: "pt-BR" },
		);
		expect(res.localized).toBe(false);
		expect(res.message).toBe("Something new");
		expect(res.hint).toBe("Try later");
	});

	it("localizes lint findings, export checks, job phases, caption problems, and post issues in pt-BR", () => {
		const t = makeT("pt-BR");
		const lint = localizeLintFinding(
			t,
			{
				code: "L_EMPTY_TIMELINE",
				message: "Timeline has no items.",
				hint: "Add a clip.",
			},
			"pt-BR",
		);
		expect(lint.localized).toBe(true);
		expect(lint.message).not.toContain("Timeline");

		const check = localizeExportCheck(t, {
			code: "X_DURATION_TOO_LONG",
			message: "Too long",
			limit: "60 s",
			got: "90 s",
		});
		expect(check.localized).toBe(true);
		expect(check.text).toContain("90 s");
		expect(check.text).toContain("60 s");

		const job = localizeJobMessage(t, "Rendering (45%)");
		expect(job.localized).toBe(true);
		expect(job.text).toBe("Renderizando (45%)");

		const batchJob = localizeJobMessage(t, "Exporting 2 of 5");
		expect(batchJob.localized).toBe(true);
		expect(batchJob.text).toBe("Exportando 2 de 5");

		const caption = localizeCaptionProblem(t, {
			code: "too_long",
			message: "Too long",
			limit: 280,
			actual: 320,
		});
		expect(caption.localized).toBe(true);
		expect(caption.text).toContain("320");
		expect(caption.text).toContain("280");

		const post = localizePostIssue(t, {
			code: "required",
			path: "title",
			message: "is required",
		});
		expect(post.localized).toBe(true);
		expect(post.text).toContain("title");
	});
});
