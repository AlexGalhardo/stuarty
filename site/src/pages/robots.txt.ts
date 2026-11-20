import type { APIRoute } from "astro";
import { site } from "@/lib/site";

export const prerender = true;

export const GET: APIRoute = () =>
	// The former landing candidates stay only as backups: out of the indexes (the screenshots under /landing/ stay open).
	new Response(`User-Agent: *\nAllow: /\nDisallow: ${site.base}/landing/classic/\nDisallow: ${site.base}/landing/creative/\n\nHost: ${site.url}\nSitemap: ${site.url}/sitemap.xml\n`, {
		headers: { "Content-Type": "text/plain; charset=utf-8" },
	});
