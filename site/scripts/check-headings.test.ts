import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { checkHeadings, headingLevels, reportHeadings } from "./check-headings.ts";

function tree(files: Record<string, string>): string {
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), "headings-"));
	for (const [rel, text] of Object.entries(files)) {
		fs.mkdirSync(path.dirname(path.join(dir, rel)), { recursive: true });
		fs.writeFileSync(path.join(dir, rel), text);
	}
	return dir;
}

test("headingLevels reads the levels in order and skips fences and front matter", () => {
	const src = "---\ntitle: x\n# not a heading\n---\n# One\n\n```sh\n# a comment\n```\n## Two\r\n### Three\n#nospace\n";
	assert.deepEqual(headingLevels(src), [1, 2, 3]);
});

test("headingLevels handles a longer closing fence and tilde fences", () => {
	assert.deepEqual(headingLevels("````md\n```\n# in\n```\n````\n~~~\n# in\n~~~\n## out\n"), [2]);
});

test("checkHeadings lists a page with a different count or level, and ignores a missing twin", () => {
	const dir = tree({
		"docs/en/a.mdx": "# A\n## B\n",
		"docs/pt-br/a.mdx": "# A\n## B\n",
		"docs/en/b.mdx": "# A\n## B\n## C\n",
		"docs/pt-br/b.mdx": "# A\n## B\n",
		"docs/en/c.mdx": "# A\n## B\n",
		"docs/pt-br/c.mdx": "# A\n### B\n",
		"docs/en/d.mdx": "# A\n## B\n### C\n",
		"docs/pt-br/d.mdx": "# A\n### C\n## B\n",
		"docs/en/e.mdx": "# A\n",
	});
	assert.deepEqual(
		checkHeadings(dir).map((m) => m.page),
		["b", "c", "d"],
	);
});

test("reportHeadings exits 1 only when there is a mismatch", () => {
	assert.equal(reportHeadings([]), 0);
	assert.equal(reportHeadings([{ page: "x", english: [1], portuguese: [2] }]), 1);
});
