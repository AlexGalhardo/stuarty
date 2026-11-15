import assert from "node:assert/strict";
import { test } from "node:test";
import { basePathProblems, foreignOriginProblems, localeProblems, twinProblems } from "./check-built.ts";

const own = "alexgalhardo.github.io";

test("a script, stylesheet, font or image from another origin is refused", () => {
	const html = [
		'<script src="https://cdn.example.com/a.js"></script>',
		'<link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Inter">',
		'<img src="//img.example.com/x.png">',
	].join("");
	// Whole messages, compared for equality: no substring test on a host name.
	assert.deepEqual(foreignOriginProblems("index.html", html, own).sort(), [
		"index.html: loads //img.example.com/x.png",
		"index.html: loads https://cdn.example.com/a.js",
		"index.html: loads https://fonts.googleapis.com/css?family=Inter",
		"index.html: refers to cdn.example.com",
		"index.html: refers to fonts.googleapis.com",
	]);
});

test("own files, inert links and the SVG namespace are accepted", () => {
	const html =
		'<script src="/vukem-video/_astro/a.js"></script><a href="https://github.com/AlexGalhardo/vukem-video">repo</a><svg xmlns="http://www.w3.org/2000/svg"></svg>';
	assert.deepEqual(foreignOriginProblems("index.html", html, own), []);
});

test("the example hosts of the self-hosting pages are text, other example hosts are not", () => {
	const ok = "<p>https://video.example.com/up and http://localhost:7878 and https://caddyserver.com</p>";
	assert.deepEqual(foreignOriginProblems("index.html", ok, own), []);
	assert.deepEqual(foreignOriginProblems("index.html", "<p>https://other.example.com/x</p>", own), [
		"index.html: refers to other.example.com",
	]);
});

test("a remote @import or url() in CSS is refused", () => {
	assert.ok(
		foreignOriginProblems("a.css", '@import url("https://x.example/a.css");', own).some((p) =>
			p.includes("@import"),
		),
	);
	assert.ok(foreignOriginProblems("a.css", "a{background:url(//cdn.example/x.png)}", own).length > 0);
});

test("a root-relative link without the base path is refused", () => {
	assert.deepEqual(
		basePathProblems("a.html", '<a href="/vukem-video/docs/">x</a><img src="/icon.svg">', "/vukem-video"),
		["a.html: /icon.svg does not start with /vukem-video"],
	);
	assert.deepEqual(basePathProblems("a.html", '<a href="/docs/">x</a>', ""), []);
});

test("every page needs a clean twin and a line in llms.txt, and a listed URL must exist", () => {
	const url = "https://example.org/base";
	const llms = [`- [A](${url}/docs/a/): x`, `- [B](${url}/docs/b/): x`, `- [C](${url}/docs/c/): x`].join("\n");
	const files = ["llms.txt", "docs/a/index.html", "docs/a.md", "docs/b/index.html", "docs/d/index.html", "docs/d.md"];
	const read = (rel: string) => (rel === "llms.txt" ? llms : rel === "docs/a.md" ? "# A\n\n<Callout>x</Callout>" : "# D");

	assert.deepEqual(twinProblems(files, read, url).sort(), [
		"docs/a.md: has JSX or an import",
		"docs/b: no Markdown twin (docs/b.md)",
		"llms.txt does not list docs/d",
		"llms.txt lists docs/c, which is not in out/",
		"llms.txt lists docs/c, which is not a page",
	].sort());
});

test("a complete site has no twin problems", () => {
	const url = "https://example.org";
	const files = ["llms.txt", "docs/a/index.html", "docs/a.md"];
	const read = (rel: string) => (rel === "llms.txt" ? `- [A](${url}/docs/a/): x` : "# A");
	assert.deepEqual(twinProblems(files, read, url), []);
});

const SITE = "https://example.org";

function page(opts: { lang: string; canonical: string; alternates?: [string, string][] }): string {
	const alt = (opts.alternates ?? []).map(([l, p]) => `<link rel="alternate" hreflang="${l}" href="${SITE}/${p}/">`).join("");
	return `<html lang="${opts.lang}"><head><link rel="canonical" href="${SITE}/${opts.canonical}/">${alt}</head></html>`;
}

function site(translated: boolean): { files: string[]; read: (rel: string) => string } {
	const pages: Record<string, string> = {
		"docs/a/index.html": page({
			lang: "en",
			canonical: "docs/a",
			alternates: translated
				? [["en", "docs/a"], ["pt-BR", "pt-br/docs/a"], ["x-default", "docs/a"]]
				: [["en", "docs/a"], ["x-default", "docs/a"]],
		}),
		"pt-br/docs/a/index.html": translated
			? page({
					lang: "pt-BR",
					canonical: "pt-br/docs/a",
					alternates: [["en", "docs/a"], ["pt-BR", "pt-br/docs/a"], ["x-default", "docs/a"]],
				})
			: page({ lang: "pt-BR", canonical: "docs/a" }),
		"docs/a.md": "# A",
		"pt-br/docs/a.md": translated ? "# A" : "> Not translated yet: this is the English text.\n\n# A",
		"404.html": '<div data-lang="en"></div><div data-lang="pt-br"></div>',
		"sitemap.xml": translated ? `<loc>${SITE}/docs/a/</loc><loc>${SITE}/pt-br/docs/a/</loc>` : `<loc>${SITE}/docs/a/</loc>`,
	};
	return { files: Object.keys(pages), read: (rel) => pages[rel] ?? "" };
}

test("a translated pair names each other and a fallback claims no alternate", () => {
	const good = site(true);
	assert.deepEqual(localeProblems(good.files, good.read, SITE), []);

	const fallback = site(false);
	assert.deepEqual(localeProblems(fallback.files, fallback.read, SITE), []);
});

test("an asymmetric hreflang, a fallback with alternates and a missing twin are refused", () => {
	const broken = site(true);
	const read = (rel: string) =>
		rel === "pt-br/docs/a/index.html" ? page({ lang: "pt-BR", canonical: "pt-br/docs/a" }) : broken.read(rel);
	assert.ok(localeProblems(broken.files, read, SITE).some((p) => p.includes("hreflang en should point to docs/a")));

	const lost = site(true);
	const files = lost.files.filter((f) => f !== "pt-br/docs/a.md");
	assert.ok(localeProblems(files, lost.read, SITE).some((p) => p.includes("pt-br/docs/a.md is missing")));

	const chatty = site(false);
	const readChatty = (rel: string) =>
		rel === "pt-br/docs/a/index.html"
			? page({ lang: "pt-BR", canonical: "docs/a", alternates: [["pt-BR", "pt-br/docs/a"]] })
			: chatty.read(rel);
	assert.ok(localeProblems(chatty.files, readChatty, SITE).some((p) => p.includes("has hreflang alternates")));
});
