import assert from "node:assert/strict";
import { test } from "node:test";
import { esc } from "./generate-reference.ts";

test("braces, angle brackets and pipes cannot become MDX syntax or end a table cell", () => {
	assert.equal(esc("a {b} <c> | d"), String.raw`a \{b\} &lt;c&gt; \| d`);
});
