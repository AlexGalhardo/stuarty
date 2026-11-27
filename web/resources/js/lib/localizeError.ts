import type { Translate } from "@/lib/i18n";

// The envelope of an error returned by the core, service or engine.
export interface ErrorLike {
	readonly code: string;
	readonly message: string;
	readonly hint?: string | undefined;
	readonly details?: { readonly [key: string]: unknown } | undefined;
}

// Optional editor context used to format item names and timeline frames.
export interface ErrorTextContext {
	itemName(id: string): string;
	time(frame: number): string;
}

export interface LocalizedError {
	headline: string;
	message: string;
	hint: string | undefined;
	// False when the message is the raw server English text, so callers can mark it `lang="en"`.
	localized: boolean;
}

export interface LocalizeErrorOptions {
	ctx?: ErrorTextContext;
	// Current UI locale. When set to "en" on open-ended codes without a structured reason,
	// preserves the server's specific English sentence if present.
	locale?: string;
}

export interface LocalizedText {
	text: string;
	localized: boolean;
}

export interface LocalizedFinding {
	message: string;
	hint: string | undefined;
	localized: boolean;
}

const REASON_ALIASES: Readonly<
	Record<string, Readonly<Record<string, string>>>
> = {
	E_EXPORT_FAILED: {
		encoder: "encoder_failed",
		decoder: "decoder_failed",
		render: "render_failed",
		audio: "audio_failed",
		publish: "publish_failed",
	},
};

function num(value: unknown): number | null {
	return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function str(value: unknown): string | null {
	return typeof value === "string" && value.trim() !== "" ? value.trim() : null;
}

function record(value: unknown): { [key: string]: unknown } | null {
	return typeof value === "object" && value !== null && !Array.isArray(value)
		? (value as { [key: string]: unknown })
		: null;
}

function lookup(
	t: Translate,
	key: string,
	params?: Record<string, string | number>,
): string | undefined {
	const text = t(key, params);
	return text === key ? undefined : text;
}

// The localized headline of an error code (SDD-012 section 4.4); falls back to the generic headline.
export function errorHeadline(t: Translate, code: string): string {
	return lookup(t, `ui.editor.errors.${code}`) ?? t("ui.editor.errors.unknown");
}

function buildParams(
	details: { readonly [key: string]: unknown } | undefined,
): Record<string, string | number> {
	const params: Record<string, string | number> = {};
	if (details === undefined) return params;
	const fields = [
		"limit",
		"field",
		"provider",
		"model",
		"network",
		"codec",
		"encoder",
		"variable",
		"id",
		"scope",
		"currency",
	] as const;
	for (const name of fields) {
		const val = str(details[name]);
		if (val !== null) params[name] = val;
	}
	const numeric = ["value", "ceiling"] as const;
	for (const name of numeric) {
		const val = num(details[name]);
		if (val !== null) params[name] = val;
	}
	const retry = num(details.retry_after) ?? num(details.retry_after_seconds);
	if (retry !== null) params.seconds = retry;
	if (Array.isArray(details.findings)) params.count = details.findings.length;
	else if (Array.isArray(details.checks)) params.count = details.checks.length;
	return params;
}

function reasonOf(
	code: string,
	details: { readonly [key: string]: unknown } | undefined,
): string | null {
	if (details === undefined) return null;
	const raw = str(details.reason) ?? str(details.kind);
	if (raw === null) return null;
	const aliased = REASON_ALIASES[code]?.[raw] ?? raw;
	return /^[a-z0-9_]+$/.test(aliased) ? aliased : null;
}

// Unified error localizer shared by the dashboard, video editor, and image editor.
// Resolution order:
//   1. Parameterized editor/domain rules (E_OVERLAP, E_VALIDATION.allowed, E_OUT_OF_SOURCE, E_ID_EXISTS, E_QUEUE_FULL, E_RATE_LIMITED)
//   2. Exact sub-case key `ui.errors.reason.<code>.<reason>` (+ `ui.errors.reason_hint.<code>.<reason>`)
//   3. Code-level key `ui.errors.message.<code>` (+ `ui.errors.hint.<code>`)
//   4. Last-resort fallback to the raw server English text (`localized: false`).
export function localizeError(
	t: Translate,
	error: ErrorLike,
	options: LocalizeErrorOptions = {},
): LocalizedError {
	const headline = errorHeadline(t, error.code);
	const details = error.details;
	const ctx = options.ctx;
	const params = buildParams(details);
	const codeHint = lookup(t, `ui.errors.hint.${error.code}`, params);

	const done = (message: string, hint?: string): LocalizedError => ({
		headline,
		message,
		hint,
		localized: true,
	});

	// 1. Parameterized shapes with rich context (preserves VV-126 wording).
	if (error.code === "E_OVERLAP" && ctx !== undefined) {
		const other = str(details?.conflicting_item);
		if (other !== null && details !== undefined) {
			const free = num(details.next_free);
			const msg = lookup(t, "ui.editor.errors.detail.overlap", {
				item: ctx.itemName(other),
			});
			if (msg !== undefined) {
				const hint =
					free !== null
						? lookup(t, "ui.editor.errors.detail.overlap_free", {
								time: ctx.time(free),
							})
						: lookup(t, "ui.editor.errors.detail.overlap_none");
				return done(msg, hint);
			}
		}
	}

	if (error.code === "E_VALIDATION") {
		const allowed = details?.allowed;
		if (
			Array.isArray(allowed) &&
			allowed.length > 0 &&
			allowed.every((v) => typeof v === "string")
		) {
			const msg = lookup(t, "ui.editor.errors.detail.allowed", {
				values: allowed.join(", "),
			});
			if (msg !== undefined) return done(msg, codeHint);
		}
	}

	if (error.code === "E_OUT_OF_SOURCE" && ctx !== undefined) {
		const max = num(record(details?.available)?.max_duration);
		if (max !== null && max > 0) {
			const msg = lookup(t, "ui.editor.notice.source_has_only", {
				available: ctx.time(max),
			});
			if (msg !== undefined) return done(msg, codeHint);
		}
	}

	if (error.code === "E_ID_EXISTS") {
		const id = str(details?.id);
		if (id !== null) {
			const msg = lookup(t, "ui.editor.errors.detail.id_exists", { id });
			if (msg !== undefined) return done(msg, codeHint);
		}
	}

	if (error.code === "E_QUEUE_FULL") {
		const limit = str(details?.limit);
		const value = num(details?.value);
		const ceiling = num(details?.ceiling);
		if (limit !== null && value !== null && ceiling !== null) {
			const msg = lookup(t, "ui.editor.errors.detail.queue_full", {
				limit,
				value,
				ceiling,
			});
			if (msg !== undefined) return done(msg, codeHint);
		}
	}

	if (error.code === "E_RATE_LIMITED") {
		const seconds = num(details?.retry_after);
		if (seconds !== null) {
			const msg = lookup(t, "ui.editor.errors.detail.rate_limited", {
				seconds,
			});
			if (msg !== undefined) return done(msg, codeHint);
		}
	}

	// 2. Sub-case key by details.reason / details.kind.
	const reason = reasonOf(error.code, details);
	if (reason !== null) {
		const reasonMsg = lookup(
			t,
			`ui.errors.reason.${error.code}.${reason}`,
			params,
		);
		if (reasonMsg !== undefined) {
			const reasonHint =
				lookup(t, `ui.errors.reason_hint.${error.code}.${reason}`, params) ??
				codeHint;
			return done(reasonMsg, reasonHint);
		}
	}

	// 3. In English, keep the server's specific English sentence and hint when no structured sub-case matched.
	// In all non-English locales (e.g. pt-BR), always use the localized code-level catalogue entry.
	if (options.locale === "en" && error.message.trim() !== "") {
		return {
			headline,
			message: error.message,
			hint: error.hint ?? codeHint,
			localized: true,
		};
	}

	// 4. Code-level message & hint.
	const codeMsg = lookup(t, `ui.errors.message.${error.code}`, params);
	if (codeMsg !== undefined) {
		return done(codeMsg, codeHint);
	}

	// 5. Last-resort fallback for unknown/future error codes.
	return {
		headline,
		message: error.message,
		hint: error.hint,
		localized: false,
	};
}

// Localizes a lint finding (video, image, or skill lint) by its L_* code.
export function localizeLintFinding(
	t: Translate,
	finding: {
		readonly code: string;
		readonly message: string;
		readonly hint?: string | undefined;
	},
	locale?: string,
): LocalizedFinding {
	if (locale === "en" && finding.message.trim() !== "") {
		return {
			message: finding.message,
			hint: finding.hint,
			localized: true,
		};
	}
	const message = lookup(t, `ui.lint.message.${finding.code}`);
	if (message === undefined) {
		return {
			message: finding.message,
			hint: finding.hint,
			localized: false,
		};
	}
	const hint =
		finding.hint !== undefined
			? lookup(t, `ui.lint.hint.${finding.code}`)
			: undefined;
	return {
		message,
		hint,
		localized: true,
	};
}

// Localizes an export preset check finding (X_*) by its code.
export function localizeExportCheck(
	t: Translate,
	finding: {
		readonly code: string;
		readonly message: string;
		readonly limit: string;
		readonly got: string;
	},
	locale?: string,
): LocalizedText {
	if (locale === "en" && finding.message.trim() !== "") {
		return { text: finding.message, localized: true };
	}
	const text = lookup(t, `ui.export_check.message.${finding.code}`, {
		limit: finding.limit,
		got: finding.got,
	});
	return text === undefined
		? { text: finding.message, localized: false }
		: { text, localized: true };
}

const EXACT_JOB_PHASES: Readonly<Record<string, string>> = {
	"Writing the bundle": "writing_bundle",
	"Rendering the frame": "rendering_frame",
	"starting the face helper": "starting_face_helper",
	"looking for faces": "looking_for_faces",
	"writing the face track": "writing_face_track",
	peaks: "peaks",
	uploading: "uploading",
	"checking the provider": "checking_provider",
	transcribing: "transcribing",
	"writing the transcript": "writing_transcript",
	probing: "probing",
	normalizing: "normalizing",
	proxy: "proxy",
	thumbnails: "thumbnails",
	silence: "silence",
	loudness: "loudness",
	speech: "speech",
	beats: "beats",
};

// Localizes known job progress phase messages sent by the service.
export function localizeJobMessage(
	t: Translate,
	message: string,
): LocalizedText {
	const trimmed = message.trim();
	const exact = EXACT_JOB_PHASES[trimmed];
	if (exact !== undefined) {
		const text = lookup(t, `ui.jobs.phase.${exact}`);
		if (text !== undefined) return { text, localized: true };
	}
	const renderMatch = /^Rendering \((\d+)%\)$/.exec(trimmed);
	if (renderMatch !== null) {
		const text = lookup(t, "ui.jobs.phase.rendering_percent", {
			percent: renderMatch[1] ?? "0",
		});
		if (text !== undefined) return { text, localized: true };
	}
	const batchMatch = /^Exporting (\d+) of (\d+)$/.exec(trimmed);
	if (batchMatch !== null) {
		const text = lookup(t, "ui.jobs.phase.exporting_of", {
			current: batchMatch[1] ?? "1",
			total: batchMatch[2] ?? "1",
		});
		if (text !== undefined) return { text, localized: true };
	}
	return { text: message, localized: false };
}

// Localizes a caption validation problem from core/writing.
export function localizeCaptionProblem(
	t: Translate,
	problem: {
		readonly code: string;
		readonly message: string;
		readonly limit?: number;
		readonly actual?: number;
	},
): LocalizedText {
	const params: Record<string, string | number> = {};
	if (typeof problem.limit === "number") params.limit = problem.limit;
	if (typeof problem.actual === "number") params.actual = problem.actual;
	const text = lookup(t, `ui.write.caption_problem.${problem.code}`, params);
	return text === undefined
		? { text: problem.message, localized: false }
		: { text, localized: true };
}

// Localizes a post validation issue from core/writing.
export function localizePostIssue(
	t: Translate,
	issue: {
		readonly code: string;
		readonly path: string;
		readonly message: string;
	},
): LocalizedText {
	const text = lookup(t, `ui.write.post_issue.${issue.code}`, {
		path: issue.path,
	});
	return text === undefined
		? { text: `${issue.path}: ${issue.message}`, localized: false }
		: { text, localized: true };
}
