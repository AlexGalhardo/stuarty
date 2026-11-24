import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import Section from "./Section";

describe("inspector Section", () => {
	test("is an open region with its header as a button that controls the body", () => {
		const html = renderToString(
			<Section title="Transform" name="transform">
				<p>fields</p>
			</Section>,
		);
		expect(html).toContain('data-section="transform"');
		expect(html).toContain('aria-expanded="true"');
		expect(html).toContain("aria-controls=");
		expect(html).not.toContain('hidden=""');
	});

	test("starts closed, with the body hidden but in the document, when it asks to", () => {
		const html = renderToString(
			<Section title="Transform" defaultOpen={false}>
				<p>fields</p>
			</Section>,
		);
		expect(html).toContain('aria-expanded="false"');
		expect(html).toContain('hidden=""');
		expect(html).toContain("fields");
	});
});
