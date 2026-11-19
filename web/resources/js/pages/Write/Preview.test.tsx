import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import MarkdownPreview, { blocks } from "./Preview";

function html(text: string): string {
	return renderToString(
		<MarkdownPreview text={text} label="Preview" empty="Nothing to show" />,
	);
}

describe("the blocks of a text", () => {
	test("headings, paragraphs, lists and code", () => {
		const text =
			"# Hook\nline one\nline two\n\n- a\n- b\n\n1. first\n2) second\n\n```\n<b>raw</b>\n```\nend";
		expect(blocks(text)).toEqual([
			{ kind: "heading", level: 1, text: "Hook" },
			{ kind: "paragraph", text: "line one line two" },
			{ kind: "list", ordered: false, items: ["a", "b"] },
			{ kind: "list", ordered: true, items: ["first", "second"] },
			{ kind: "code", text: "<b>raw</b>" },
			{ kind: "paragraph", text: "end" },
		]);
	});

	test("a heading of seven marks, or with no space, is a paragraph", () => {
		expect(blocks("####### no\n\n#no")).toEqual([
			{ kind: "paragraph", text: "####### no" },
			{ kind: "paragraph", text: "#no" },
		]);
	});

	test("an empty text has no block", () => {
		expect(blocks("")).toEqual([]);
		expect(blocks("\n\n  \n")).toEqual([]);
	});
});

describe("the preview", () => {
	test("markup in the text is shown as text and builds no element", () => {
		const evil =
			'<img src=x onerror="window.__pwned=1"> <script>alert(1)</script>\n\n[a link](javascript:alert(1))';
		const out = html(evil);
		expect(out).not.toContain("<img");
		expect(out).not.toContain("<script");
		expect(out).not.toContain("<a ");
		expect(out).toContain("&lt;img src=x onerror=");
		expect(out).toContain("&lt;script&gt;alert(1)&lt;/script&gt;");
		expect(out).toContain("[a link](javascript:alert(1))");
	});

	test("markup in a heading, a list and a code block is shown as text too", () => {
		const out = html("# <b>h</b>\n\n- <i>x</i>\n\n```\n<u>y</u>\n```");
		for (const tag of ["<b>", "<i>", "<u>"]) expect(out).not.toContain(tag);
		expect(out).toContain("&lt;b&gt;h&lt;/b&gt;");
		expect(out).toContain("&lt;i&gt;x&lt;/i&gt;");
		expect(out).toContain("&lt;u&gt;y&lt;/u&gt;");
	});

	test("emphasis and code get their elements, and nothing else does", () => {
		const out = html("a **bold** and *soft* and `code` and _under_");
		expect(out).toContain("<strong>bold</strong>");
		expect(out.match(/<em>/g)?.length).toBe(2);
		expect(out).toContain("<code");
	});

	test("a heading is a paragraph, so the outline of the page is not broken", () => {
		const out = html("# Big\n\n## Medium");
		expect(out).not.toMatch(/<h[1-6]/);
		expect(out).toContain("Big");
	});

	test("an empty text says so, in a labelled region", () => {
		const out = html("");
		expect(out).toContain("Nothing to show");
		expect(out).toContain('aria-label="Preview"');
	});
});
