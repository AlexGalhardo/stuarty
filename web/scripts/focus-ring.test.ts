import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const css = readFileSync(join(web, "resources/css/app.css"), "utf8").replace(
	/\r\n/g,
	"\n",
);

describe("focus ring", () => {
	test("a slider draws the 2 px ring on its thumb, whose hidden input holds the focus", () => {
		expect(css).toMatch(
			/\[data-slot="slider-thumb"\]:has\(input:focus-visible\) \{\n\toutline: 2px solid var\(--color-focus\);/,
		);
	});
});
