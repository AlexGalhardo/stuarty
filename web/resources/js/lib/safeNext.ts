// Only a path of this site is a place to go back to after the login. The value
// is resolved against a fixed placeholder origin, the way the browser would
// resolve it, so `//host`, `/\host` and other parser tricks that land on
// another origin are ignored. Backslashes and control characters are refused
// first, since browsers drop or rewrite them while resolving.
const base = "http://vukem-video.invalid";

export function safeNext(next: string | null): string {
	const fallback = "/projects";
	if (next === null || !next.startsWith("/")) return fallback;
	// biome-ignore lint/suspicious/noControlCharactersInRegex: refusing them is the point
	if (/[\\\u0000-\u001f\u007f]/.test(next)) return fallback;

	let url: URL;
	try {
		url = new URL(next, base);
	} catch {
		return fallback;
	}
	if (url.origin !== base) return fallback;
	return url.pathname + url.search + url.hash;
}
