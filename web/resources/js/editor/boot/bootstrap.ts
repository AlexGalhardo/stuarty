import type { Message } from "@/lib/i18n";

// The object the editor route writes into the page as JSON (SDD-012 section
// 2.2). The server owns it; the editor only validates its shape.
export interface EditorBootstrap {
	project_id: string;
	mode: "local" | "network";
	api_base: string;
	locale: string;
	locales: string[];
	messages: Record<string, Message>;
	user: { id: string; name: string } | null;
	phase: "prototype" | "alpha" | "beta";
	version: string;
}

const PROJECT_ID = /^prj_[0-9a-hjkmnp-tv-z]{10}$/;

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function parseBootstrap(
	text: string | null | undefined,
): EditorBootstrap {
	if (!text) throw new Error("editor bootstrap is missing");
	const raw: unknown = JSON.parse(text);
	if (!isRecord(raw)) throw new Error("editor bootstrap is not an object");
	const { project_id, mode, api_base, locale, messages, phase } = raw;
	if (typeof project_id !== "string" || !PROJECT_ID.test(project_id)) {
		throw new Error("editor bootstrap has no valid project_id");
	}
	if (mode !== "local" && mode !== "network") {
		throw new Error("editor bootstrap has an unknown mode");
	}
	if (typeof api_base !== "string" || !api_base.startsWith("/")) {
		throw new Error("editor bootstrap has no api_base");
	}
	if (typeof locale !== "string" || !isRecord(messages)) {
		throw new Error("editor bootstrap has no catalog");
	}
	if (phase !== "prototype" && phase !== "alpha" && phase !== "beta") {
		throw new Error("editor bootstrap has an unknown phase");
	}
	return {
		project_id,
		mode,
		api_base,
		locale,
		locales: Array.isArray(raw.locales) ? (raw.locales as string[]) : [locale],
		messages: messages as EditorBootstrap["messages"],
		user: isRecord(raw.user) ? (raw.user as EditorBootstrap["user"]) : null,
		phase,
		version: typeof raw.version === "string" ? raw.version : "",
	};
}

// The actor string the core records for this tab (SDD-012 section 5.8).
export function ownActor(bootstrap: EditorBootstrap): string {
	return bootstrap.mode === "network" && bootstrap.user
		? `user:${bootstrap.user.id}`
		: "local:ui";
}
