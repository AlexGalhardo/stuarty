import assert from "node:assert/strict";
import { test } from "node:test";
import { basePathProblems, foreignOriginProblems, twinProblems } from "./check-built.ts";

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
