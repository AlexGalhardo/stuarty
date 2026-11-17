import type { Translate } from "@/lib/i18n";

// What an error needs to be worded in the catalogue: the envelope of the core
// without its English text.
export interface ErrorLike {
	readonly code: string;
	readonly message: string;
	readonly hint?: string | undefined;
	readonly details?: { readonly [key: string]: unknown } | undefined;
}

// What the editor knows that the error does not say: the display name of an
// item and how a frame is written (the project fps and the time display).
export interface ErrorTextContext {
	itemName(id: string): string;
	time(frame: number): string;
}

export interface ErrorText {
	message: string;
	hint: string | undefined;
	// False when the text is the server's English as received, so the caller can
	// mark it `lang="en"`.
	localized: boolean;
}

const STATIC_BODIES: Readonly<Record<string, string>> = {
	E_NOTHING_TO_UNDO: "ui.editor.errors.detail.nothing_to_undo",
	E_NOTHING_TO_REDO: "ui.editor.errors.detail.nothing_to_redo",
	E_ASSET_NOT_READY: "ui.editor.errors.detail.asset_not_ready",
	E_UNAUTHORIZED: "ui.editor.errors.detail.unauthorized",
	E_FORBIDDEN: "ui.editor.errors.detail.forbidden",
	E_PATH_NOT_ALLOWED: "ui.editor.errors.detail.path_not_allowed",
	E_FILE_NOT_FOUND: "ui.editor.errors.detail.file_not_found",
	E_FFMPEG_MISSING: "ui.editor.errors.detail.ffmpeg_missing",
	E_NO_ENCODER: "ui.editor.errors.detail.no_encoder",
	E_WORKSPACE_LOCKED: "ui.editor.errors.detail.workspace_locked",
	E_PAYLOAD_TOO_LARGE: "ui.editor.errors.detail.payload_too_large",
	E_CANCELLED: "ui.editor.errors.detail.cancelled",
};

function num(value: unknown): number | null {
	return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function record(value: unknown): { [key: string]: unknown } | null {
	return typeof value === "object" && value !== null && !Array.isArray(value)
		? (value as { [key: string]: unknown })
		: null;
}

// The catalogue wording of an error, from its code and its `details` (counts,
// frames, names); a code or a shape with no entry keeps the server's English
// message and hint (VV-126). Only the wording changes: every value comes from
// the core.
export function errorText(
	t: Translate,
	error: ErrorLike,
	ctx: ErrorTextContext,
): ErrorText {
	const asReceived: ErrorText = {
		message: error.message,
		hint: error.hint,
		localized: false,
	};
	const details = error.details;
	const localized = (message: string, hint?: string): ErrorText => ({
		message,
		hint,
		localized: true,
	});

	const staticKey = STATIC_BODIES[error.code];
	if (staticKey !== undefined) return localized(t(staticKey));

	switch (error.code) {
		case "E_OVERLAP": {
			const other = details?.conflicting_item;
			if (typeof other !== "string" || details === undefined) {
				return asReceived;
			}
			const free = num(details.next_free);
			return localized(
				t("ui.editor.errors.detail.overlap", { item: ctx.itemName(other) }),
				free !== null
					? t("ui.editor.errors.detail.overlap_free", { time: ctx.time(free) })
					: t("ui.editor.errors.detail.overlap_none"),
			);
		}
		case "E_VALIDATION": {
			const allowed = details?.allowed;
			if (
				Array.isArray(allowed) &&
				allowed.length > 0 &&
				allowed.every((v) => typeof v === "string")
			) {
				return localized(
					t("ui.editor.errors.detail.allowed", { values: allowed.join(", ") }),
				);
			}
			return asReceived;
		}
		case "E_OUT_OF_SOURCE": {
			const max = num(record(details?.available)?.max_duration);
			return max === null || max <= 0
				? asReceived
				: localized(
						t("ui.editor.notice.source_has_only", { available: ctx.time(max) }),
					);
		}
		case "E_ID_EXISTS": {
			const id = details?.id;
			return typeof id === "string"
				? localized(t("ui.editor.errors.detail.id_exists", { id }))
				: asReceived;
		}
		case "E_QUEUE_FULL": {
			const limit = details?.limit;
			const value = num(details?.value);
			const ceiling = num(details?.ceiling);
			return typeof limit === "string" && value !== null && ceiling !== null
				? localized(
						t("ui.editor.errors.detail.queue_full", {
							limit,
							value,
							ceiling,
						}),
					)
				: asReceived;
		}
		case "E_RATE_LIMITED": {
			const seconds = num(details?.retry_after);
			return seconds === null
				? asReceived
				: localized(t("ui.editor.errors.detail.rate_limited", { seconds }));
		}
		default:
			return asReceived;
	}
}
