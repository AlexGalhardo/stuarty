// Only a path of this site is a place to go back to after the login: a `next`
// that names another origin (`//host`, `https://...`, `/\host`) is ignored.
export function safeNext(next: string | null): string {
	if (next === null || !next.startsWith("/")) return "/projects";
	if (next.startsWith("//") || next.startsWith("/\\")) return "/projects";
	return next;
}
