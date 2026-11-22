import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { ReleaseMarkView, releaseLabel } from "./ReleaseMark";

describe("releaseLabel", () => {
	test("prefixes the server version", () => {
		expect(releaseLabel("0.28.0")).toBe("v0.28.0");
		expect(releaseLabel("v0.28.0")).toBe("v0.28.0");
		expect(releaseLabel("  ")).toBe("");
	});
});

describe("ReleaseMarkView", () => {
	test("draws the release at the bottom right", () => {
		const html = renderToString(<ReleaseMarkView version="0.28.0" />);
		expect(html).toContain('data-testid="release-mark"');
		expect(html).toContain("v0.28.0");
		expect(html).toContain("bottom-[max(0.25rem");
		expect(html).toContain("right-[max(1rem");
	});

	test("draws nothing until the server answers", () => {
		expect(renderToString(<ReleaseMarkView version="" />)).toBe("");
	});
});
