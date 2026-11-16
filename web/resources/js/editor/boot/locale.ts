import { writeLocale } from "../state/prefs";
import { deepLinkQuery } from "./deepLink";

// Changing the language of the editor (SDD-012 section 16.3): the choice is
// kept in this browser (localStorage, through prefs) and the page is loaded
// again with `?lang=`, which the server reads before the stored header the
// Inertia pages send. The playhead and the selected item travel in the query
// (`?t=`, `?item=`) so the person comes back to the same place.

export interface LocaleDeps {
	store(locale: string): void;
	assign(url: string): void;
	pathname: string;
}

const defaults = (): LocaleDeps => ({
	store: writeLocale,
	assign: (url) => window.location.assign(url),
	pathname: window.location.pathname,
});

function withLang(query: string, locale: string): string {
	const params = new URLSearchParams(query);
	params.set("lang", locale);
	return `?${params.toString()}`;
}

export function switchLocale(
	locale: string,
	place: { frame: number; item: string | null },
	deps: LocaleDeps = defaults(),
): void {
	deps.store(locale);
	deps.assign(
		deps.pathname + withLang(deepLinkQuery(place.frame, place.item), locale),
	);
}

// The page is rendered before the browser can say which language it prefers.
// When the stored choice differs from the rendered one, the URL to load again
// with `?lang=`; null when nothing is to be done (also when that was tried
// already, so a stored language the server does not have cannot loop).
export function reconcileUrl(
	rendered: string,
	stored: string | null,
	location: { pathname: string; search: string },
): string | null {
	if (stored === null || stored.toLowerCase() === rendered.toLowerCase())
		return null;
	if (new URLSearchParams(location.search).has("lang")) return null;
	return location.pathname + withLang(location.search, stored);
}
