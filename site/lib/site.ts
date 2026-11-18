/** The site is served from `origin + base`: a GitHub Pages project site lives under /vukem-video. */
const base = import.meta.env.BASE_URL.replace(/\/$/, "");

export const site = {
	name: "Vukem Video",
	origin: import.meta.env.SITE as string,
	base,
	url: `${import.meta.env.SITE}${base}`,
	repo: "https://github.com/AlexGalhardo/vukem-video",
	releases: "https://github.com/AlexGalhardo/vukem-video/releases",
	editBase: "https://github.com/AlexGalhardo/vukem-video/edit/main/site/content",
} as const;

/** Prefixes an internal path (`/docs/...`, `/icon.svg`) with the base path; anything else is returned as is. */
export function withBase(href: string): string {
	if (!href.startsWith("/") || href.startsWith("//") || base === "") return href;
	return `${base}${href}`;
}
