import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { esc, serviceStatuses } from "./generate-reference.ts";

test("braces, angle brackets and pipes cannot become MDX syntax or end a table cell", () => {
	assert.equal(esc("a {b} <c> | d"), String.raw`a \{b\} &lt;c&gt; \| d`);
});

test("the service status table is read from service/error.go", () => {
	const status = serviceStatuses();
	assert.equal(status.get("E_UNAUTHORIZED"), 401);
	assert.equal(status.get("E_WORKSPACE_LOCKED"), 503);
	assert.equal(status.get("E_QUEUE_FULL"), 429);
	assert.equal(status.get("E_LIMIT_EXCEEDED"), 422);
	assert.equal(status.get("E_CLIENT_CLOSED"), 499);
	assert.equal(status.get("E_EXPORT_FAILED"), undefined, "a job failure has no entry: it answers 500");
});

// The title and the description of a page are shown on every page (the sidebar, the search, llms.txt), so the
// former name of the product may be only in the body of the migration page (epic 40, SDD-010 section 15.5).
test("no title or description of a page says the former name", () => {
	const content = path.join(import.meta.dirname, "..", "content");
	const walk = (dir: string): string[] =>
		fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
	for (const file of walk(content).filter((f) => f.endsWith(".mdx"))) {
		const head = fs.readFileSync(file, "utf8").match(/^---\r?\n([\s\S]*?)\r?\n---/)?.[1] ?? "";
		assert.doesNotMatch(head, /vukem[ -]video/i, path.relative(content, file));
	}
});
