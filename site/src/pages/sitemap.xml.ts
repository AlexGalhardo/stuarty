import type { APIRoute } from "astro";
import { getAllDocs, getDoc, isTranslated } from "@/lib/content";
import { htmlLang, localizePath, locales } from "@/lib/locale";
import { site } from "@/lib/site";

export const prerender = true;

function escapeXml(value: string): string {
	return value
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;")
		.replace(/'/g, "&apos;");
}

type Entry = { path: string; lastmod?: string; priority: string; alternates: { lang: string; path: string }[] };

const at = (path: string) => `${site.url}${path.endsWith("/") ? path : `${path}/`}`;

/** The alternates of a page that exists in both locales, with an x-default on the English one (section 4.3). */
function pair(englishPath: string): Entry["alternates"] {
	return [
		...locales.map((l) => ({ lang: htmlLang[l], path: localizePath(l, englishPath) })),
		{ lang: "x-default", path: englishPath },
	];
}

/** Every page that has text of its own in its locale, with xhtml:link alternates for translated pairs. */
export const GET: APIRoute = () => {
	const entries: Entry[] = [
		{ path: "/", priority: "1.0", alternates: [] },
	];

	for (const locale of locales) {
		for (const doc of getAllDocs(locale)) {
			if (!isTranslated(doc)) continue;
			const other = getDoc(locale === "en" ? "pt-br" : "en", doc.slug);
			const both = other !== undefined && isTranslated(other);
			entries.push({
				path: doc.href,
				lastmod: doc.updated,
				priority: locale === "en" ? "0.8" : "0.7",
				alternates: both ? pair(doc.englishHref) : [],
			});
		}
	}

	entries.push(
		{ path: "/changelog/", priority: "0.5", alternates: pair("/changelog") },
		{ path: "/pt-br/changelog/", priority: "0.4", alternates: pair("/changelog") },
	);

	const body = [
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">`,
		...entries.map((e) => {
			const lastmod = e.lastmod ? `<lastmod>${escapeXml(e.lastmod)}</lastmod>` : "";
			const links = e.alternates
				.map((a) => `<xhtml:link rel="alternate" hreflang="${a.lang}" href="${escapeXml(at(a.path))}"/>`)
				.join("");
			return `<url><loc>${escapeXml(at(e.path))}</loc>${lastmod}<changefreq>weekly</changefreq><priority>${e.priority}</priority>${links}</url>`;
		}),
		`</urlset>`,
	].join("");

	return new Response(body, { headers: { "Content-Type": "application/xml; charset=utf-8" } });
};
