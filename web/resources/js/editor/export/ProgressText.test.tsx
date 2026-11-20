import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import ProgressText from "./ProgressText";

// renderToString puts a comment between the text nodes of `{percent}%`.
const html = (node: React.ReactNode): string =>
	renderToString(node).replaceAll("<!-- -->", "");

describe("ProgressText", () => {
	test("shows the exact percent to the eye and hides it from a screen reader", () => {
		const out = html(<ProgressText progress={0.42} message="Encoding" />);
		expect(out).toMatch(/<p aria-hidden="true"[^>]*>42%/);
		expect(out).toContain("Encoding");
	});

	test("says the percent to a screen reader only in steps of 10", () => {
		const said = (progress: number): string =>
			html(<ProgressText progress={progress} />).match(
				/<p class="sr-only"[^>]*>([^<]*)%<\/p>/,
			)?.[1] ?? "";
		expect([0.09, 0.1, 0.19, 0.42, 0.99, 1].map(said)).toEqual([
			"0",
			"10",
			"10",
			"40",
			"90",
			"100",
		]);
	});
});
