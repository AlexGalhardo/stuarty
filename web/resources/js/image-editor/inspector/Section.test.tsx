import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";
import { Section } from "./fields";
import { adjustCount } from "./LookFields";

describe("image inspector Section", () => {
	test("is an open region whose heading holds the button that controls the body", () => {
		const html = renderToString(
			<Section title="Mask" testId="mask">
				<p>fields</p>
			</Section>,
		);
		expect(html).toMatch(/<h3[^>]*><button[^>]*aria-expanded="true"/);
		expect(html).toContain("aria-controls=");
		expect(html).not.toContain('hidden=""');
		expect(html).toContain('data-testid="mask"');
	});

	test("starts closed with the body hidden but in the document, so a draft is kept", () => {
		const html = renderToString(
			<Section title="Filters" defaultOpen={false}>
				<p>fields</p>
			</Section>,
		);
		expect(html).toContain('aria-expanded="false"');
		expect(html).toContain('hidden=""');
		expect(html).toContain("fields");
	});

	test("says in its header what the section already holds, only when it holds something", () => {
		const withBadge = renderToString(
			<Section title="Filters" badge="2 in use">
				<p>fields</p>
			</Section>,
		);
		expect(withBadge).toContain("2 in use");
		expect(withBadge).toContain('data-testid="image-section-badge"');
		const without = renderToString(
			<Section title="Filters">
				<p>fields</p>
			</Section>,
		);
		expect(without).not.toContain("image-section-badge");
	});
});

describe("adjustCount", () => {
	test("counts the adjustments the read model lists", () => {
		expect(adjustCount(undefined)).toBe(0);
		expect(adjustCount({})).toBe(0);
		expect(
			adjustCount({
				brightness: 10,
				levels: { black: 0, white: 255, gamma: 100 },
				contrast: undefined,
			}),
		).toBe(2);
	});
});
