import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import ReleaseMark, { ReleaseMarkView, releaseLabel } from "./ReleaseMark";

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

describe("ReleaseMarkView inline", () => {
	test("is a line of the sidebar footer, not a fixed mark", () => {
		const html = renderToString(<ReleaseMarkView version="0.28.0" inline />);
		expect(html).toContain('data-testid="release-mark"');
		expect(html).toContain("v0.28.0");
		expect(html).not.toContain("fixed");
	});
});

describe("ReleaseMarkView compact", () => {
	test("wraps the tag within the collapsed rail at the standard size (VV-437)", () => {
		const rail = renderToString(
			<ReleaseMarkView version="0.28.0" inline compact />,
		);
		const wide = renderToString(<ReleaseMarkView version="0.28.0" inline />);
		expect(rail).toContain("text-xs");
		expect(rail).toContain("wrap-anywhere");
		expect(wide).not.toContain("wrap-anywhere");
	});
});

describe("ReleaseMark with a known release (VV-444)", () => {
	test("draws the shared release and asks GET /system for nothing", () => {
		const html = renderToString(<ReleaseMark known="0.58.0" />);
		expect(html).toContain("v0.58.0");
	});
});
