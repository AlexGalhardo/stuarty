import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "astro/config";
import { navigation, pagePath } from "./lib/page-files.ts";

const root = path.dirname(fileURLToPath(import.meta.url));

// SITE_URL is the public address. The default is the custom domain on GitHub Pages, so the base is "/".
// An address with a path (a project site, https://<user>.github.io/<repo>) sets that path as the base
// (SDD-010 section 12.4).
const siteUrl = new URL(process.env.SITE_URL || "https://stuarty.com");
const prefix = siteUrl.pathname.replace(/\/$/, "");
const base = prefix || "/";

// The Portuguese docs were removed (English only): every old /pt-br/docs/... address sends to the English page.
// A dynamic redirect cannot carry the base path, so each page is listed.
const legacyPortuguese = Object.fromEntries(
	navigation.flatMap((cat) =>
		cat.pages.map((page) => {
			const href = pagePath(cat, page);
			return [`/pt-br${href}`, `${prefix}${href}/`];
		}),
	),
);

// The editing commands moved from /docs/reference/commands/<name> to the pages of the edit tool. Their names are in
// generated/reference-nav.json, written by scripts/generate-reference.ts.
const navFile = path.join(root, "generated", "reference-nav.json");
const operations = fs.existsSync(navFile) ? JSON.parse(fs.readFileSync(navFile, "utf8")).mcp?.operations ?? [] : [];
const movedCommands = Object.fromEntries([
	["/docs/reference/commands", `${prefix}/docs/mcp/tools/edit/`],
	...operations.map((name) => [`/docs/reference/commands/${name}`, `${prefix}/docs/mcp/tools/edit/${name}/`]),
]);

// Static HTML for GitHub Pages. The docs root opens the introduction.
export default defineConfig({
	site: siteUrl.origin,
	base,
	output: "static",
	outDir: "out",
	trailingSlash: "always",
	build: { format: "directory" },
	server: { port: 3000 },
	integrations: [react()],
	vite: {
		plugins: [tailwindcss()],
		resolve: {
			alias: { "@": root },
		},
	},
	redirects: {
		// The destination is not rewritten with the base path by Astro, so it is written in full.
		"/docs": `${prefix}/docs/introduction/`,
		"/pt-br": `${prefix}/`,
		"/pt-br/docs": `${prefix}/docs/introduction/`,
		"/pt-br/changelog": `${prefix}/changelog/`,
		// The viewport candidate became the landing page at the root.
		"/landing/viewport": `${prefix}/`,
		// The HTTP API moved to its own section (docs/api).
		"/docs/reference/http-api": `${prefix}/docs/api/overview/`,
		...movedCommands,
		...legacyPortuguese,
	},
});
