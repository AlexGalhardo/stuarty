import { http, router } from "@inertiajs/react";

import { readLocale } from "@/editor/state/prefs";

// The language is a preference of this browser (SDD-012 section 16.3): it is
// kept in localStorage and sent on every request in this header, which the
// server reads before Accept-Language.
export const LANG_HEADER = "Vukem-Video-Lang";

// The first page of a visit is rendered before the browser can say which
// language it prefers; this asks for it again when the stored choice differs.
export function localeMismatch(
	rendered: string,
	stored: string | null,
): boolean {
	return stored !== null && stored.toLowerCase() !== rendered.toLowerCase();
}

// Adds the header to every Inertia request.
export function installLangHeader(): void {
	http.onRequest((config) => {
		const stored = readLocale();
		return stored === null
			? config
			: { ...config, headers: { ...config.headers, [LANG_HEADER]: stored } };
	});
}

// Once the app is mounted, reloads the first page in the stored language.
export function reconcileLang(): void {
	if (localeMismatch(document.documentElement.lang, readLocale())) {
		router.reload();
	}
}
