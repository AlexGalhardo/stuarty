import assert from "node:assert/strict";
import { test } from "node:test";
import { basePathProblems, foreignOriginProblems } from "./check-built.ts";

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
