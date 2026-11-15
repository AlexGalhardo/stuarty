import { deepLinkQuery } from "./deepLink";

// Changing the language of the editor (SDD-012 section 16.3): the choice is
// posted to /locale, which keeps it in the lang cookie, and the page is loaded
// again. The playhead and the selected item travel in the query (`?t=`, `?item=`)
// so the person comes back to the same place.

export interface LocaleDeps {
	post(locale: string): Promise<boolean>;
	assign(url: string): void;
	pathname: string;
}

// `redirect: "manual"`: the route answers with a redirect to a page the editor
// does not need; the cookie is already set.
async function postLocale(locale: string): Promise<boolean> {
	try {
		const response = await fetch("/locale", {
			method: "POST",
			credentials: "same-origin",
			redirect: "manual",
			headers: {
				Accept: "application/json",
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ locale }),
		});
		return response.ok || response.type === "opaqueredirect";
	} catch {
		return false;
	}
}

const defaults = (): LocaleDeps => ({
	post: postLocale,
	assign: (url) => window.location.assign(url),
	pathname: window.location.pathname,
});

// Returns whether the page is being reloaded.
export async function switchLocale(
	locale: string,
	place: { frame: number; item: string | null },
	deps: LocaleDeps = defaults(),
): Promise<boolean> {
	if (!(await deps.post(locale))) return false;
	deps.assign(deps.pathname + deepLinkQuery(place.frame, place.item));
	return true;
}
