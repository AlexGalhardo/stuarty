import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "astro/config";

const root = path.dirname(fileURLToPath(import.meta.url));

// SITE_URL is the public address. The default is the custom domain on GitHub Pages, so the base is "/".
// An address with a path (a project site, https://<user>.github.io/<repo>) sets that path as the base
// (SDD-010 section 12.4).
const siteUrl = new URL(process.env.SITE_URL || "https://video.vukem.dev");
const prefix = siteUrl.pathname.replace(/\/$/, "");
const base = prefix || "/";

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
		"/pt-br/docs": `${prefix}/pt-br/docs/introduction/`,
	},
});
