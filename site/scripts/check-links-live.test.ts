import assert from "node:assert/strict";
import test from "node:test";
import { extractLinks, pageIds } from "./check-links-live.ts";

const page = "https://example.org/stuarty/docs/guide/a/";

test("internal links are resolved, split from the fragment and kept apart from external ones", () => {
	const html = `<a href="../b/#x">b</a> <a class="c" href="/stuarty/docs/c/">c</a>
		<a href="https://github.com/o/r">g</a> <a href="mailto:a@b.c">m</a> <a href="#top">t</a>`;
	const { internal, external } = extractLinks(html, page);
	assert.deepEqual(
		internal.map((l) => [l.url, l.fragment]),
		[
			["https://example.org/stuarty/docs/guide/b/", "x"],
			["https://example.org/stuarty/docs/c/", ""],
			[page, "top"],
		],
	);
	assert.deepEqual(external, ["https://github.com/o/r"]);
});

test("page ids come from id attributes", () => {
	assert.deepEqual([...pageIds('<h2 id="one">x</h2><div id="two"></div>')], ["one", "two"]);
});
